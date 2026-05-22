package merkletree

import (
	"bytes"
	"fmt"
	"hash"
	"math/bits"
	"runtime"
	"sync"
	"sync/atomic"
)

// FixedDepthMerkleTree is an in-memory fixed-depth Merkle tree optimized for
// batch insertion followed by batch reads. The depth is fixed at construction
// time to match circuit constraints for proof verification.
//
// Usage pattern (two-phase):
//  1. Set leaves concurrently (Set only stores leaf values, no hashing)
//  2. Call Build() to compute all internal nodes bottom-up in parallel
//  3. Call GetProof/Root to read proofs (concurrent reads, no locks)
//
// Constraints:
//   - depth must be <= 32 (enforced by constructor).
//   - Multiple Set→Build cycles are correct but NOT incremental: leavesDirty bits
//     are never cleared, so each Build() recomputes all dirty nodes from scratch.
//   - GetProof/Root only reflect Sets that were followed by a Build() call.
type FixedDepthMerkleTree struct {
	depth      int
	hashSize   int
	capacity   int
	nilHashes  [][]byte // nilHashes[level] = hash of empty subtree at that level
	hasherFunc func() hash.Hash

	// leaves is a flat byte buffer indexed by key.
	// Leaf at key k is stored at leaves[k*hashSize : (k+1)*hashSize].
	// leavesDirty tracks which keys have been set.
	// Different goroutines write to different keys, so no lock is needed.
	leaves      []byte
	leavesDirty []uint64

	// levels[l] is a flat byte buffer for internal level l (1..depth).
	// Node at position p is stored at levels[l][p*hashSize : (p+1)*hashSize].
	// Only positions marked in levelDirty are valid.
	levels [][]byte

	// levelDirty[l] is a bitset tracking which positions at level l have
	// been computed (i.e., differ from nilHashes[l]).
	levelDirty [][]uint64

	// root is set after Build().
	root []byte
}

// --- Bitset utilities ---

// bitsetLen returns the number of uint64 words needed for n bits.
func bitsetLen(n int) int {
	return (n + 63) >> 6
}

// bitGet returns true if bit i is set in the bitset.
func bitGet(bs []uint64, i uint32) bool {
	return bs[i>>6]&(1<<(i&63)) != 0
}

// bitSetTrue sets bit i in the bitset (not concurrent-safe).
func bitSetTrue(bs []uint64, i uint32) {
	bs[i>>6] |= 1 << (i & 63)
}

// atomicBitSetTrue sets bit i in the bitset using atomic OR (concurrent-safe).
func atomicBitSetTrue(bs []uint64, i uint32) {
	mask := uint64(1) << (i & 63)
	for {
		old := atomic.LoadUint64(&bs[i>>6])
		if atomic.CompareAndSwapUint64(&bs[i>>6], old, old|mask) {
			return
		}
	}
}

// isAllZero returns true if the bitset has no bits set.
func isAllZero(bs []uint64) bool {
	for _, w := range bs {
		if w != 0 {
			return false
		}
	}
	return true
}

// propagateDirty builds the parent-level dirty bitset from a child-level bitset.
// Each bit i in the parent is set if bit 2i or 2i+1 is set in the child.
// This is equivalent to: parent[i] = child[2i] | child[2i+1].
// Implemented efficiently as: for each word w in child, compress adjacent pairs
// by OR, then scatter into the parent word at the correct position.
func propagateDirty(child []uint64) []uint64 {
	parentLen := bitsetLen(len(child) * 32) // each child word covers 64 bits = 32 parent bits
	parent := make([]uint64, parentLen)
	for i, w := range child {
		if w == 0 {
			continue
		}
		// Compress 64 child bits into 32 parent bits:
		// parent bit j = child bit 2j | child bit 2j+1
		compressed := compressBits(w)
		// These 32 parent bits land at parent bit offset i*32.
		// Since parentBitOffset = i*32, bitIdx is always 0 (i even) or 32 (i odd).
		// compressed is uint32 (at most 32 bits), so the shift never overflows uint64.
		parentBitOffset := uint(i) * 32
		wordIdx := parentBitOffset >> 6
		bitIdx := parentBitOffset & 63
		parent[wordIdx] |= uint64(compressed) << bitIdx
	}
	return parent
}

// compressBits takes 64 bits and compresses adjacent pairs with OR into 32 bits.
// Output bit j = input bit 2j | input bit 2j+1.
func compressBits(w uint64) uint32 {
	// OR adjacent bits: bit 2j | bit 2j+1
	ored := w | (w >> 1)
	// Now extract even-positioned bits (bit 0, 2, 4, ...) from ored
	// Using parallel bit extract: PEXT-like operation via bit manipulation
	ored &= 0x5555555555555555 // keep even bits
	// Compress even bits to contiguous positions
	ored = (ored | (ored >> 1)) & 0x3333333333333333
	ored = (ored | (ored >> 2)) & 0x0f0f0f0f0f0f0f0f
	ored = (ored | (ored >> 4)) & 0x00ff00ff00ff00ff
	ored = (ored | (ored >> 8)) & 0x0000ffff0000ffff
	ored = (ored | (ored >> 16)) & 0x00000000ffffffff
	return uint32(ored)
}

// NewFixedDepthMerkleTree creates a new in-memory fixed-depth Merkle tree.
// capacity: the number of leaf slots to pre-allocate (keys must be in [0, capacity)).
func NewFixedDepthMerkleTree(depth int, nilLeafHash []byte, hasherFunc func() hash.Hash, capacity int) *FixedDepthMerkleTree {
	if depth > 32 {
		panic("depth too large")
	}
	if depth <= 0 {
		panic("depth must be positive")
	}
	if uint64(capacity) > (uint64(1) << uint(depth)) {
		panic("capacity exceeds maximum for given depth")
	}
	hashSize := len(nilLeafHash)
	t := &FixedDepthMerkleTree{
		depth:       depth,
		hashSize:    hashSize,
		capacity:    capacity,
		hasherFunc:  hasherFunc,
		leaves:      make([]byte, capacity*hashSize),
		leavesDirty: make([]uint64, bitsetLen(capacity)),
		levels:      make([][]byte, depth+1), // levels[0] unused; levels[1..depth] for internal nodes
		levelDirty:  make([][]uint64, depth+1),
	}

	// Precompute nilHashes for each level.
	t.nilHashes = make([][]byte, depth+1)
	t.nilHashes[0] = make([]byte, hashSize)
	copy(t.nilHashes[0], nilLeafHash)

	h := hasherFunc()
	for i := 1; i <= depth; i++ {
		h.Reset()
		h.Write(t.nilHashes[i-1])
		h.Write(t.nilHashes[i-1])
		t.nilHashes[i] = h.Sum(nil)
	}

	t.root = copyBytes(t.nilHashes[depth])
	return t
}

// Set stores a leaf value at the given key. This only writes the leaf;
// internal nodes are NOT computed until Build() is called.
// Safe to call concurrently from multiple goroutines (different keys).
func (t *FixedDepthMerkleTree) Set(key uint32, value []byte) error {
	if int(key) >= t.capacity {
		return fmt.Errorf("key %d out of range for capacity %d", key, t.capacity)
	}
	offset := int(key) * t.hashSize
	copy(t.leaves[offset:offset+t.hashSize], value)
	atomicBitSetTrue(t.leavesDirty, key)
	return nil
}

// Build computes all internal nodes bottom-up in parallel.
// Must be called after all Set operations are complete.
// After Build, GetProof and Root return correct values.
func (t *FixedDepthMerkleTree) Build() {
	hashSize := t.hashSize

	// Build initial dirty bitset for level 1 from leavesDirty.
	// Dirty at level 1: parent position = leafIndex >> 1.
	dirtyBits := propagateDirty(t.leavesDirty)

	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}

	for level := 1; level <= t.depth; level++ {
		if isAllZero(dirtyBits) {
			break
		}

		// Allocate level buffer and assign dirtyBits as this level's dirty bitset.
		bufSize := len(dirtyBits) * 64 // max positions covered
		t.levels[level] = make([]byte, bufSize*hashSize)
		t.levelDirty[level] = dirtyBits

		// Process dirty positions in parallel.
		// Split work by bitset words for good locality.
		numWords := len(dirtyBits)
		wordsPerWorker := (numWords + workers - 1) / workers

		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wordStart := w * wordsPerWorker
			wordEnd := wordStart + wordsPerWorker
			if wordEnd > numWords {
				wordEnd = numWords
			}
			if wordStart >= wordEnd {
				break
			}

			wg.Add(1)
			go func(wordStart, wordEnd, level int) {
				defer wg.Done()
				h := t.hasherFunc()
				buf := t.levels[level]

				for wi := wordStart; wi < wordEnd; wi++ {
					word := dirtyBits[wi]
					if word == 0 {
						continue
					}
					basePos := uint32(wi) << 6
					for word != 0 {
						// Find next set bit
						tz := uint32(bits.TrailingZeros64(word))
						pos := basePos + tz
						word &= word - 1 // clear lowest set bit

						left := t.getNodeAt(level-1, pos<<1)
						right := t.getNodeAt(level-1, (pos<<1)|1)

						h.Reset()
						h.Write(left)
						h.Write(right)

						// h.Sum appends the hash to the provided slice (per hash.Hash contract).
						// buf[offset:offset] has length 0 but sufficient capacity, so Sum
						// writes directly into buf at the correct position without allocation.
						offset := int(pos) * hashSize
						h.Sum(buf[offset:offset])
					}
				}
			}(wordStart, wordEnd, level)
		}
		wg.Wait()

		// Propagate dirty to next level.
		if level < t.depth {
			dirtyBits = propagateDirty(dirtyBits)
		}
	}

	// Set root.
	if len(t.levelDirty[t.depth]) > 0 && bitGet(t.levelDirty[t.depth], 0) {
		t.root = make([]byte, hashSize)
		copy(t.root, t.levels[t.depth][0:hashSize])
	} else {
		t.root = copyBytes(t.nilHashes[t.depth])
	}
}

// Root returns the current root hash.
func (t *FixedDepthMerkleTree) Root() []byte {
	return t.root
}

// Get returns the leaf value at the given key. Returns nilHashes[0] if not set.
func (t *FixedDepthMerkleTree) Get(key uint32) []byte {
	if int(key) >= t.capacity || !bitGet(t.leavesDirty, key) {
		return copyBytes(t.nilHashes[0])
	}
	offset := int(key) * t.hashSize
	return copyBytes(t.leaves[offset : offset+t.hashSize])
}

// GetProof returns the Merkle proof (sibling hashes) for the given key.
// Must be called after Build().
func (t *FixedDepthMerkleTree) GetProof(key uint32) ([][]byte, error) {
	if uint64(key) >= (uint64(1) << uint(t.depth)) {
		return nil, fmt.Errorf("key %d out of range for tree depth %d", key, t.depth)
	}
	proof := make([][]byte, t.depth)
	pos := key
	for level := 0; level < t.depth; level++ {
		proof[level] = copyBytes(t.getNodeAt(level, pos^1))
		pos >>= 1
	}
	return proof, nil
}

// getNodeAt returns the hash at (level, position).
// NOTE: returns a direct slice into the internal buffer (no copy) for performance.
// Callers in Build() only pass the result to h.Write() (read-only).
// Callers in GetProof() wrap the result with copyBytes().
// Any new caller that modifies the returned slice will corrupt the tree.
func (t *FixedDepthMerkleTree) getNodeAt(level int, position uint32) []byte {
	if level == 0 {
		if int(position) < t.capacity && bitGet(t.leavesDirty, position) {
			offset := int(position) * t.hashSize
			return t.leaves[offset : offset+t.hashSize]
		}
		return t.nilHashes[0]
	}
	if t.levelDirty[level] != nil {
		wordIdx := position >> 6
		if int(wordIdx) < len(t.levelDirty[level]) && bitGet(t.levelDirty[level], position) {
			offset := int(position) * t.hashSize
			return t.levels[level][offset : offset+t.hashSize]
		}
	}
	return t.nilHashes[level]
}

// VerifyProof verifies a Merkle proof for the given key and leaf.
func VerifyProof(root []byte, key uint32, proof [][]byte, leaf []byte, depth int, hasherFunc func() hash.Hash) bool {
	if len(proof) != depth || uint64(key) >= (uint64(1)<<uint(depth)) {
		return false
	}
	h := hasherFunc()
	node := make([]byte, len(leaf))
	copy(node, leaf)

	for i := 0; i < depth; i++ {
		bit := key & (1 << i)
		h.Reset()
		if bit == 0 {
			h.Write(node)
			h.Write(proof[i])
		} else {
			h.Write(proof[i])
			h.Write(node)
		}
		node = h.Sum(nil)
	}
	return bytes.Equal(node, root)
}

func copyBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
