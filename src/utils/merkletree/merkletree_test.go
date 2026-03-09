package merkletree

import (
	"bytes"
	"hash"
	"math/big"
	"sync"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
)

func nilAccountHash() []byte {
	zero := &fr.Element{0, 0, 0, 0}
	h := poseidon.Poseidon(zero, zero, zero, zero, zero).Bytes()
	return h[:]
}

func newTestTree(capacity int) *FixedDepthMerkleTree {
	return NewFixedDepthMerkleTree(28, nilAccountHash(), func() hash.Hash {
		return poseidon.NewPoseidon()
	}, capacity)
}

func newHasherFunc() func() hash.Hash {
	return func() hash.Hash {
		return poseidon.NewPoseidon()
	}
}

func makeLeafValue(k uint64) []byte {
	var e fr.Element
	e.SetBigInt(new(big.Int).SetUint64(k + 1))
	b := e.Bytes()
	return b[:]
}

func TestNewFixedDepthMerkleTree(t *testing.T) {
	tree := newTestTree(100)
	if tree == nil {
		t.Fatal("tree should not be nil")
	}
	root := tree.Root()
	if root == nil {
		t.Fatal("root should not be nil")
	}
	if len(root) != 32 {
		t.Fatalf("root should be 32 bytes, got %d", len(root))
	}
	if !bytes.Equal(root, tree.nilHashes[28]) {
		t.Fatal("root of empty tree should equal nilHashes[28]")
	}
}

func TestSetBuildAndRoot(t *testing.T) {
	tree := newTestTree(100)
	emptyRoot := tree.Root()

	tree.Set(0, makeLeafValue(0))
	tree.Build()

	newRoot := tree.Root()
	if bytes.Equal(newRoot, emptyRoot) {
		t.Fatal("root should change after Set+Build")
	}
	if len(newRoot) != 32 {
		t.Fatalf("root should be 32 bytes, got %d", len(newRoot))
	}
}

func TestSetMultipleKeys(t *testing.T) {
	tree := newTestTree(1000)

	values := make([][]byte, 3)
	for i := uint32(0); i < 3; i++ {
		values[i] = makeLeafValue(uint64(i))
		tree.Set(i, values[i])
	}
	tree.Build()

	for i := uint32(0); i < 3; i++ {
		got := tree.Get(i)
		if !bytes.Equal(got, values[i]) {
			t.Fatalf("key %d: got different value than what was set", i)
		}
	}

	got := tree.Get(999)
	if !bytes.Equal(got, tree.nilHashes[0]) {
		t.Fatal("unset key should return nilHash[0]")
	}
}

func TestGetProof(t *testing.T) {
	tree := newTestTree(100)
	tree.Set(5, makeLeafValue(42))
	tree.Build()

	proof, err := tree.GetProof(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(proof) != 28 {
		t.Fatalf("proof should have 28 elements, got %d", len(proof))
	}
	for i, p := range proof {
		if len(p) != 32 {
			t.Fatalf("proof[%d] should be 32 bytes, got %d", i, len(p))
		}
	}
}

func TestGetProofVerify(t *testing.T) {
	tree := newTestTree(100000)
	keys := []uint32{0, 5, 100, 50000}
	for _, k := range keys {
		err := tree.Set(k, makeLeafValue(uint64(k)))
		if err != nil {
			t.Fatalf("Set failed for key %d: %v", k, err)
		}
	}
	tree.Build()

	root := tree.Root()
	for _, k := range keys {
		proof, err := tree.GetProof(k)
		if err != nil {
			t.Fatal(err)
		}
		leaf := tree.Get(k)
		if !VerifyProof(root, k, proof, leaf, 28, newHasherFunc()) {
			t.Fatalf("proof verification failed for key %d", k)
		}
	}

	// Verify proof for an empty key.
	emptyKeys := []uint32{999, 99999}
	for _, k := range emptyKeys {
		proof, err := tree.GetProof(k)
		if err != nil {
			t.Fatal(err)
		}
		emptyLeaf := tree.Get(k)
		if !VerifyProof(root, k, proof, emptyLeaf, 28, newHasherFunc()) {
			t.Fatalf("proof verification failed for empty key %d", k)
		}
	}
}

func TestConcurrentSet(t *testing.T) {
	numKeys := 1000
	tree := newTestTree(numKeys)

	leafValues := make([][]byte, numKeys)
	for i := 0; i < numKeys; i++ {
		leafValues[i] = makeLeafValue(uint64(i))
	}

	var wg sync.WaitGroup
	wg.Add(numKeys)
	for i := 0; i < numKeys; i++ {
		go func(k int) {
			defer wg.Done()
			tree.Set(uint32(k), leafValues[k])
		}(i)
	}
	wg.Wait()

	tree.Build()

	for i := 0; i < numKeys; i++ {
		got := tree.Get(uint32(i))
		if !bytes.Equal(got, leafValues[i]) {
			t.Fatalf("key %d: got different value than what was set", i)
		}
	}

	root := tree.Root()
	for i := 0; i < numKeys; i++ {
		proof, err := tree.GetProof(uint32(i))
		if err != nil {
			t.Fatal(err)
		}
		if !VerifyProof(root, uint32(i), proof, leafValues[i], 28, newHasherFunc()) {
			t.Fatalf("proof verification failed for key %d", i)
		}
	}
}

func TestConcurrentSetAndRead(t *testing.T) {
	numKeys := 200
	tree := newTestTree(numKeys)

	var wg sync.WaitGroup
	wg.Add(numKeys)
	for i := 0; i < numKeys; i++ {
		go func(k int) {
			defer wg.Done()
			tree.Set(uint32(k), makeLeafValue(uint64(k)))
		}(i)
	}
	wg.Wait()

	tree.Build()

	// Concurrent reads after Build.
	wg.Add(numKeys)
	for i := 0; i < numKeys; i++ {
		go func(k int) {
			defer wg.Done()
			_ = tree.Get(uint32(k))
			_, _ = tree.GetProof(uint32(k))
			_ = tree.Root()
		}(i)
	}
	wg.Wait()
}

func TestSequentialKeysConcurrent(t *testing.T) {
	numKeys := 10000
	tree := newTestTree(numKeys)

	leafValues := make([][]byte, numKeys)
	for i := 0; i < numKeys; i++ {
		leafValues[i] = makeLeafValue(uint64(i))
	}

	var wg sync.WaitGroup
	wg.Add(numKeys)
	for i := 0; i < numKeys; i++ {
		go func(k int) {
			defer wg.Done()
			tree.Set(uint32(k), leafValues[k])
		}(i)
	}
	wg.Wait()

	tree.Build()

	root := tree.Root()
	for i := 0; i < numKeys; i++ {
		proof, err := tree.GetProof(uint32(i))
		if err != nil {
			t.Fatal(err)
		}
		if !VerifyProof(root, uint32(i), proof, leafValues[i], 28, newHasherFunc()) {
			t.Fatalf("proof verification failed for sequential key %d", i)
		}
	}
}

func TestCapacityOverflowCheck(t *testing.T) {
	// depth=32 means max capacity is 1<<32 = 4294967296.
	// Ensure the overflow-safe check works correctly.
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for capacity exceeding max")
		}
	}()
	// depth=4 means max capacity is 16; passing 17 should panic.
	NewFixedDepthMerkleTree(4, make([]byte, 32), func() hash.Hash {
		return poseidon.NewPoseidon()
	}, 17)
}

func TestCapacityAtMax(t *testing.T) {
	// depth=4 means max capacity is 16; exactly 16 should not panic.
	tree := NewFixedDepthMerkleTree(4, make([]byte, 32), func() hash.Hash {
		return poseidon.NewPoseidon()
	}, 16)
	if tree == nil {
		t.Fatal("tree should not be nil")
	}
}

func BenchmarkSet(b *testing.B) {
	tree := newTestTree(b.N + 1)
	val := makeLeafValue(0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.Set(uint32(i), val)
	}
}

func BenchmarkBuild(b *testing.B) {
	for i := 0; i < 1; i++ {
		b.StopTimer()
		leavesCount := 1 << 27 // 134,217,728
		tree := newTestTree(leavesCount)
		for j := 0; j < leavesCount; j++ {
			tree.Set(uint32(j), makeLeafValue(uint64(j)))
		}
		b.StartTimer()
		tree.Build()
	}
}

func BenchmarkGetProof(b *testing.B) {
	tree := newTestTree(10000)
	for i := 0; i < 10000; i++ {
		tree.Set(uint32(i), makeLeafValue(uint64(i)))
	}
	tree.Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.GetProof(uint32(i % 10000))
	}
}

// --- Bitset utility tests ---

func TestBitsetLen(t *testing.T) {
	cases := []struct {
		n    int
		want int
	}{
		{0, 0}, {1, 1}, {63, 1}, {64, 1}, {65, 2}, {128, 2}, {129, 3},
	}
	for _, c := range cases {
		got := bitsetLen(c.n)
		if got != c.want {
			t.Errorf("bitsetLen(%d) = %d, want %d", c.n, got, c.want)
		}
	}
}

func TestBitGetSet(t *testing.T) {
	bs := make([]uint64, 4)
	// Initially all bits should be unset.
	for i := uint32(0); i < 256; i++ {
		if bitGet(bs, i) {
			t.Fatalf("bit %d should not be set initially", i)
		}
	}
	// Set some specific bits.
	indices := []uint32{0, 1, 63, 64, 127, 128, 200, 255}
	for _, idx := range indices {
		bitSetTrue(bs, idx)
	}
	for _, idx := range indices {
		if !bitGet(bs, idx) {
			t.Fatalf("bit %d should be set", idx)
		}
	}
	// Check a non-set bit.
	if bitGet(bs, 100) {
		t.Fatal("bit 100 should not be set")
	}
}

func TestIsAllZero(t *testing.T) {
	bs := make([]uint64, 4)
	if !isAllZero(bs) {
		t.Fatal("empty bitset should be all zero")
	}
	bitSetTrue(bs, 130)
	if isAllZero(bs) {
		t.Fatal("bitset with a set bit should not be all zero")
	}
}

func TestCompressBits(t *testing.T) {
	// compressBits: output bit j = input bit 2j | input bit 2j+1
	// Input: bits 0,1 set => output bit 0 set
	if compressBits(0b11) != 1 {
		t.Fatal("compressBits(0b11) should be 1")
	}
	// Input: bit 0 only => output bit 0 set
	if compressBits(0b01) != 1 {
		t.Fatal("compressBits(0b01) should be 1")
	}
	// Input: bit 1 only => output bit 0 set
	if compressBits(0b10) != 1 {
		t.Fatal("compressBits(0b10) should be 1")
	}
	// Input: bits 2,3 set => output bit 1 set
	if compressBits(0b1100) != 0b10 {
		t.Fatalf("compressBits(0b1100) = %b, want 0b10", compressBits(0b1100))
	}
	// Zero input
	if compressBits(0) != 0 {
		t.Fatal("compressBits(0) should be 0")
	}
	// All bits set: 64 input bits => 32 output bits, all set
	if compressBits(^uint64(0)) != ^uint32(0) {
		t.Fatalf("compressBits(all ones) = %032b, want all ones", compressBits(^uint64(0)))
	}
}

func TestPropagateDirty(t *testing.T) {
	// Single word, bits 0 and 2 set => parent bits 0 and 1 set
	child := []uint64{0b101}
	parent := propagateDirty(child)
	if !bitGet(parent, 0) {
		t.Fatal("parent bit 0 should be set")
	}
	if !bitGet(parent, 1) {
		t.Fatal("parent bit 1 should be set")
	}
	if bitGet(parent, 2) {
		t.Fatal("parent bit 2 should not be set")
	}

	// Two words: bit 64 set in child => parent bit 32
	child2 := make([]uint64, 2)
	bitSetTrue(child2, 64)
	parent2 := propagateDirty(child2)
	if !bitGet(parent2, 32) {
		t.Fatal("parent bit 32 should be set (child bit 64)")
	}
}

