package utils

import (
	// "encoding/hex"
	"fmt"
	"os"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
	// "github.com/stretchr/testify/assert"
	"encoding/csv"
	"math/big"
	"testing"
)

func ComputeAssetsCommitmentForTest(userAssets []AccountAsset) []byte {
	nEles := (AssetCounts*5 + 2) / 3
	flattenUserAssets := make([]uint64, 3*nEles)
	for i := 0; i < AssetCounts; i++ {
		flattenUserAssets[5*i] = userAssets[i].Equity
		flattenUserAssets[5*i+1] = userAssets[i].Debt
		flattenUserAssets[5*i+2] = userAssets[i].Loan
		flattenUserAssets[5*i+3] = userAssets[i].Margin
		flattenUserAssets[5*i+4] = userAssets[i].PortfolioMargin
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
		testUserAssets1[i].Loan = uint64(i*10 + 100)
		testUserAssets1[i].Margin = uint64(i*10 + 100)
		testUserAssets1[i].PortfolioMargin = uint64(i*10 + 100)

		userAssets[testUserAssets1[i].Index].Equity = testUserAssets1[i].Equity
		userAssets[testUserAssets1[i].Index].Debt = testUserAssets1[i].Debt
		userAssets[testUserAssets1[i].Index].Loan = testUserAssets1[i].Loan
		userAssets[testUserAssets1[i].Index].Margin = testUserAssets1[i].Margin
		userAssets[testUserAssets1[i].Index].PortfolioMargin = testUserAssets1[i].PortfolioMargin
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
		testUserAssets1[i].Loan = uint64(i*10 + 100)
		testUserAssets1[i].Margin = uint64(i*10 + 100)
		testUserAssets1[i].PortfolioMargin = uint64(i*10 + 100)

		userAssets[testUserAssets1[i].Index].Equity = testUserAssets1[i].Equity
		userAssets[testUserAssets1[i].Index].Debt = testUserAssets1[i].Debt
		userAssets[testUserAssets1[i].Index].Loan = testUserAssets1[i].Loan
		userAssets[testUserAssets1[i].Index].Margin = testUserAssets1[i].Margin
		userAssets[testUserAssets1[i].Index].PortfolioMargin = testUserAssets1[i].PortfolioMargin
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

func TestParseUserDataSet(t *testing.T) {
	// two user files: one has 90 valid accounts and 10 invalid accounts,
	// the other has 80 valid accounts and 20 invalid accounts
	accounts, cexAssetsInfo, _ := ParseUserDataSet("../sampledata")
	// if err != nil {
	// 	t.Errorf("error: %s\n", err.Error())
	// }
	totalNum := 0
	for _, v := range accounts {
		totalNum += len(v)
	}
	if totalNum != 170 {
		t.Errorf("error: %d\n", totalNum)
	}

	_ = cexAssetsInfo
	accounts0, invalidAccountNum, _ := ReadUserDataFromCsvFile("../sampledata/sample_users0.csv", cexAssetsInfo)
	totalNum = 0
	for _, v := range accounts0 {
		totalNum += len(v)
	}
	if invalidAccountNum != 10 {
		t.Errorf("error: %d\n", invalidAccountNum)
	}
	if totalNum != 90 {
		t.Errorf("error: %d\n", totalNum)
	}
	accounts1, invalidAccountNum, _ := ReadUserDataFromCsvFile("../sampledata/sample_users1.csv", cexAssetsInfo)
	totalNum = 0
	for _, v := range accounts1 {
		totalNum += len(v)
	}
	if invalidAccountNum != 20 {
		t.Errorf("error: %d\n", invalidAccountNum)
	}
	if totalNum != 80 {
		t.Errorf("error: %d\n", totalNum)
	}

}

func TestParseCexAssetInfoFromFile(t *testing.T) {
	cf, err := os.Open("./cex_assets_info.csv")
	if err != nil {
		t.Errorf("error: %s\n", err.Error())
	}
	defer cf.Close()
	csvReader := csv.NewReader(cf)
	data, err := csvReader.ReadAll()
	if err != nil {
		t.Error(err.Error())
	}
	data = data[1:]
	assetIndexes := make([]string, len(data))
	for i, d := range data {
		assetIndexes[i] = d[0]
	}
	fmt.Println("assetIndexes: ", len(assetIndexes))
	cexAssetsInfo, err := ParseCexAssetInfoFromFile("./cex_assets_info.csv", assetIndexes)
	if err != nil {
		t.Errorf("error: %s\n", err.Error())
	}
	actualAssetsCount := 0
	for _, v := range cexAssetsInfo {
		if v.Symbol != "reserved" {
			actualAssetsCount++
		}
	}
	if actualAssetsCount != 326 {
		t.Errorf("error: %d\n", actualAssetsCount)
	}
	fmt.Println("cexAssetsInfo: ", cexAssetsInfo[0].PortfolioMarginRatios)
}
