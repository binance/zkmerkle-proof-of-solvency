package utils

import (
	"hash"

	"github.com/binance/zkmerkle-proof-of-solvency/src/utils/merkletree"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
)

var (
	NilAccountHash []byte
)

func NewAccountTree(capacity int) (*merkletree.FixedDepthMerkleTree, error) {
	return merkletree.NewFixedDepthMerkleTree(
		AccountTreeDepth,
		NilAccountHash,
		func() hash.Hash {
			return poseidon.NewPoseidon()
		},
		capacity,
	), nil
}

func VerifyMerkleProof(root []byte, accountIndex uint32, proof [][]byte, node []byte) bool {
	return merkletree.VerifyProof(root, accountIndex, proof, node, AccountTreeDepth, func() hash.Hash {
		return poseidon.NewPoseidon()
	})
}
