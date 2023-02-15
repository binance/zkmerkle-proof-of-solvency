package utils

import (
	"encoding/hex"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
	"github.com/stretchr/testify/assert"
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

func TestReadUserDataFromCsvFile(t *testing.T) {
	accounts, cexAssetsInfo, err := ReadUserDataFromCsvFile("../sampledata/sample_users0.csv")
	assert.Equal(t, err, nil)
	assert.Equal(t, len(accounts), 100)
	assert.Equal(t, len(cexAssetsInfo), AssetCounts)
}

func TestConvertAssetInfoToBytesPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	ConvertAssetInfoToBytes(1)
}

func TestConvertAssetInfoToBytes(t *testing.T) {
	cexAssets := CexAssetInfo{
		TotalEquity: 10,
		TotalDebt:   1,
		BasePrice:   1,
		Symbol:      "BTC",
		Index:       0,
	}
	b := ConvertAssetInfoToBytes(cexAssets)
	// hex(3402823669209384634652192818391391666177) = 0x0a00000000000000010000000000000001
	assert.Equal(t, hex.EncodeToString(b), "0a00000000000000010000000000000001")
}

func TestParseUserDataSet(t *testing.T) {
	accounts, cexAssetsInfo, err := ParseUserDataSet("../sampledata")
	assert.Equal(t, err, nil)
	assert.Equal(t, len(accounts), 200)
	assert.Equal(t, len(cexAssetsInfo), 350)

	accounts0, cexAssetsInfo0, err := ReadUserDataFromCsvFile("../sampledata/sample_users0.csv")
	accounts1, cexAssetsInfo1, err := ReadUserDataFromCsvFile("../sampledata/sample_users1.csv")

	assert.Equal(t, len(accounts), len(accounts0)+len(accounts1))
	for i := 0; i < len(cexAssetsInfo); i++ {
		assert.Equal(t, cexAssetsInfo[i].TotalEquity, cexAssetsInfo0[i].TotalEquity+cexAssetsInfo1[i].TotalEquity)
		assert.Equal(t, cexAssetsInfo[i].TotalDebt, cexAssetsInfo0[i].TotalDebt+cexAssetsInfo1[i].TotalDebt)
		assert.Equal(t, cexAssetsInfo[i].BasePrice, cexAssetsInfo0[i].BasePrice)
		assert.Equal(t, cexAssetsInfo[i].BasePrice, cexAssetsInfo1[i].BasePrice)
	}
}

func TestAccountInfoToHash(t *testing.T) {
	poseidonHasher := poseidon.NewPoseidon()
	account := AccountInfo{
		AccountIndex: 0,
		AccountId:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		TotalEquity:  new(big.Int).SetInt64(0),
		TotalDebt:    new(big.Int).SetInt64(0),
		Assets:       []AccountAsset{},
	}
	emptyAccountHash := AccountInfoToHash(&account, &poseidonHasher)
	assert.Equal(t, hex.EncodeToString(emptyAccountHash), "221970e0ba2d0b02a979e616cf186305372e73aab1e74f749772c9fef54dbf91")
}

func TestComputeCexAssetsCommitment(t *testing.T) {
	cexAssetsInfo := []CexAssetInfo{}
	hash := ComputeCexAssetsCommitment(cexAssetsInfo)
	assert.Equal(t, hex.EncodeToString(hash), "0c1c6be634fec4e6a30c0966ada871fe98cdaecc580d5129d704ba57b045fb81")
}

func TestRecoverAfterCexAssets(t *testing.T) {
	witness := BatchCreateUserWitness{
		BeforeCexAssets: []CexAssetInfo{
			{
				TotalEquity: 10,
				TotalDebt:   1,
				BasePrice:   1,
				Symbol:      "BTC",
				Index:       0,
			},
			{
				TotalEquity: 20,
				TotalDebt:   2,
				BasePrice:   1,
				Symbol:      "ETH",
				Index:       1,
			},
		},
		CreateUserOps: []CreateUserOperation{
			{
				Assets: []AccountAsset{
					{
						Index:  0,
						Equity: 1,
						Debt:   1,
					},
					{
						Index:  1,
						Equity: 2,
						Debt:   2,
					},
				},
			},
		},
	}

	expectAfterCexAssetsInfo := []CexAssetInfo{
		{
			TotalEquity: 11,
			TotalDebt:   2,
			BasePrice:   1,
			Symbol:      "BTC",
			Index:       0,
		},
		{
			TotalEquity: 22,
			TotalDebt:   4,
			BasePrice:   1,
			Symbol:      "ETH",
			Index:       1,
		},
	}

	hasher := poseidon.NewPoseidon()
	for i := 0; i < len(expectAfterCexAssetsInfo); i++ {
		commitment := ConvertAssetInfoToBytes(expectAfterCexAssetsInfo[i])
		hasher.Write(commitment)
	}
	cexCommitment := hasher.Sum(nil)
	witness.AfterCEXAssetsCommitment = cexCommitment
	actualCexAssetsInfo := RecoverAfterCexAssets(&witness)
	assert.Equal(t, actualCexAssetsInfo, expectAfterCexAssetsInfo)
}

func TestDecodeBatchWitness(t *testing.T) {
	data := "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"
	witness := DecodeBatchWitness(data)
	assert.Equal(t, len(witness.CreateUserOps), 2)
}
