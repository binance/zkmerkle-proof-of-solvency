package utils

import (
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
	"math/big"
	"testing"
)

func ComputeAssetsCommitmentForTest(userAssets []AccountAsset) []byte {
	nEles := (AssetCounts*2 + 2) / 3
	flattenUserAssets := make([]uint64, 3*nEles)
	for i := 0; i < AssetCounts; i++ {
		flattenUserAssets[2*i] = userAssets[i].Equity
		flattenUserAssets[2*i+1] = userAssets[i].Debt
	}

	hasher := poseidon.NewPoseidon()
	for i := 0; i < nEles; i++ {
		aBigInt := new(big.Int).SetUint64(flattenUserAssets[3*i])
		bBigInt := new(big.Int).SetUint64(flattenUserAssets[3*i+1])
		cBigInt := new(big.Int).SetUint64(flattenUserAssets[3*i+2])
		sumBigIntBytes := new(big.Int).Add(new(big.Int).Add(
			new(big.Int).Mul(aBigInt, Uint64MaxValueBigIntSquare),
			new(big.Int).Mul(bBigInt, Uint64MaxValueBigInt)),
			cBigInt).Bytes()
		hasher.Write(sumBigIntBytes)
	}
	expectHash := hasher.Sum(nil)
	return expectHash
}

func TestComputeUserAssetsCommitment(t *testing.T) {
	userAssets := make([]AccountAsset, AssetCounts)
	testUserAssets1 := make([]AccountAsset, 10)
	for i := 0; i < 10; i++ {
		testUserAssets1[i].Index = uint16(3 * i)
		testUserAssets1[i].Equity = uint64(i*10 + 1000)
		testUserAssets1[i].Debt = uint64(i*10 + 500)
		userAssets[testUserAssets1[i].Index].Equity = testUserAssets1[i].Equity
		userAssets[testUserAssets1[i].Index].Debt = testUserAssets1[i].Debt
	}
	for i := 0; i < AssetCounts; i++ {
		userAssets[i].Index = uint16(i)
	}
	expectHash := ComputeAssetsCommitmentForTest(userAssets)

	hasher := poseidon.NewPoseidon()
	hasher.Reset()
	actualHash := ComputeUserAssetsCommitment(&hasher, testUserAssets1)
	if string(expectHash) != string(actualHash) {
		t.Errorf("not match: %x:%x\n", expectHash, actualHash)
	}

	// case 2
	userAssets = make([]AccountAsset, AssetCounts)
	for i := 0; i < AssetCounts; i++ {
		userAssets[i].Index = uint16(i)
	}
	for i := 0; i < 10; i++ {
		testUserAssets1[i].Index = uint16(3*i) + 2
		testUserAssets1[i].Equity = uint64(i*10 + 1000)
		testUserAssets1[i].Debt = uint64(i*10 + 500)
		userAssets[testUserAssets1[i].Index].Equity = testUserAssets1[i].Equity
		userAssets[testUserAssets1[i].Index].Debt = testUserAssets1[i].Debt
	}

	expectHash = ComputeAssetsCommitmentForTest(userAssets)

	hasher.Reset()
	actualHash = ComputeUserAssetsCommitment(&hasher, testUserAssets1)
	if string(expectHash) != string(actualHash) {
		t.Errorf("not match: %x:%x\n", expectHash, actualHash)
	}

	// case 2
	userAssets = make([]AccountAsset, AssetCounts)
	for i := 0; i < AssetCounts; i++ {
		userAssets[i].Index = uint16(i)
		userAssets[i].Equity = uint64(i*10 + 1000)
		userAssets[i].Debt = uint64(i*10 + 500)
	}
	expectHash = ComputeAssetsCommitmentForTest(userAssets)
	hasher.Reset()
	actualHash = ComputeUserAssetsCommitment(&hasher, userAssets)
	if string(expectHash) != string(actualHash) {
		t.Errorf("not match: %x:%x\n", expectHash, actualHash)
	}

}
