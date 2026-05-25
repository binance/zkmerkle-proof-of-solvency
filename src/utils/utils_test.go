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
	target := GetAssetsCountOfUser(userAssets)
	nEles := (target*6 + 2) / 3
	flattenUserAssets := make([]uint64, 3*nEles)
	for i := 0; i < target; i++ {
		flattenUserAssets[6*i] = uint64(userAssets[i].Index)
		flattenUserAssets[6*i+1] = userAssets[i].Equity
		flattenUserAssets[6*i+2] = userAssets[i].Debt
		flattenUserAssets[6*i+3] = userAssets[i].Loan
		flattenUserAssets[6*i+4] = userAssets[i].Margin
		flattenUserAssets[6*i+5] = userAssets[i].PortfolioMargin
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
	testUserAssets1 := make([]AccountAsset, 10)
	for i := 0; i < 10; i++ {
		testUserAssets1[i].Index = uint16(3*i + 30)
		testUserAssets1[i].Equity = uint64(i*10 + 1000)
		testUserAssets1[i].Debt = uint64(i*10 + 500)
		testUserAssets1[i].Loan = uint64(i*10 + 100)
		testUserAssets1[i].Margin = uint64(i*10 + 100)
		testUserAssets1[i].PortfolioMargin = uint64(i*10 + 100)
	}
	target := GetAssetsCountOfUser(testUserAssets1)
	paddingCounts := target - len(testUserAssets1)
	userAssets := make([]AccountAsset, target)
	currentPaddingCounts := 0
	currentAssetIndex := 0
	index := 0
	for i := 0; i < len(testUserAssets1); i++ {
		if currentPaddingCounts < paddingCounts {
			for j := currentAssetIndex; j < int(testUserAssets1[i].Index); j++ {
				userAssets[index].Index = uint16(j)
				currentPaddingCounts++
				index++
				if currentPaddingCounts >= paddingCounts {
					break
				}
			}
		}
		userAssets[index].Index = testUserAssets1[i].Index
		userAssets[index].Equity = testUserAssets1[i].Equity
		userAssets[index].Debt = testUserAssets1[i].Debt
		userAssets[index].Loan = testUserAssets1[i].Loan
		userAssets[index].Margin = testUserAssets1[i].Margin
		userAssets[index].PortfolioMargin = testUserAssets1[i].PortfolioMargin
		index++
		currentAssetIndex = int(testUserAssets1[i].Index) + 1
	}
	for i := index; i < target; i++ {
		userAssets[i].Index = uint16(currentAssetIndex)
		currentAssetIndex++
	}
	expectHash := ComputeAssetsCommitmentForTest(userAssets)

	hasher := poseidon.NewPoseidon()
	hasher.Reset()
	actualHash := ComputeUserAssetsCommitment(&hasher, testUserAssets1)
	if string(expectHash) != string(actualHash) {
		t.Errorf("not match: %x:%x\n", expectHash, actualHash)
	}

	// case 2
	testUserAssets2 := make([]AccountAsset, 100)
	target = GetAssetsCountOfUser(testUserAssets2)
	userAssets = make([]AccountAsset, target)
	for i := 0; i < target; i++ {
		userAssets[i].Index = uint16(i)
	}
	for i := 0; i < 100; i++ {
		testUserAssets2[i].Index = uint16(3*i) + 2
		testUserAssets2[i].Equity = uint64(i*10 + 1000)
		testUserAssets2[i].Debt = uint64(i*10 + 500)
		testUserAssets2[i].Loan = uint64(i*10 + 100)
		testUserAssets2[i].Margin = uint64(i*10 + 100)
		testUserAssets2[i].PortfolioMargin = uint64(i*10 + 100)

		userAssets[testUserAssets2[i].Index].Equity = testUserAssets2[i].Equity
		userAssets[testUserAssets2[i].Index].Debt = testUserAssets2[i].Debt
		userAssets[testUserAssets2[i].Index].Loan = testUserAssets2[i].Loan
		userAssets[testUserAssets2[i].Index].Margin = testUserAssets2[i].Margin
		userAssets[testUserAssets2[i].Index].PortfolioMargin = testUserAssets2[i].PortfolioMargin
	}

	expectHash = ComputeAssetsCommitmentForTest(userAssets)

	hasher.Reset()
	actualHash = ComputeUserAssetsCommitment(&hasher, testUserAssets2)
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
	if actualAssetsCount != 483 {
		t.Errorf("error: %d\n", actualAssetsCount)
	}
	fmt.Println("cexAssetsInfo: ", cexAssetsInfo[0].PortfolioMarginRatios)
}

func TestParseTiers(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[int]int
		wantErr bool
	}{
		{
			name:  "normal two tiers",
			input: "500:4,50:20",
			want:  map[int]int{500: 4, 50: 20},
		},
		{
			name:  "single tier",
			input: "500:4",
			want:  map[int]int{500: 4},
		},
		{
			name:  "with spaces",
			input: " 500 : 4 , 50 : 20 ",
			want:  map[int]int{500: 4, 50: 20},
		},
		{
			name:    "invalid format",
			input:   "500-4",
			wantErr: true,
		},
		{
			name:    "non-numeric key",
			input:   "abc:4",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTiers(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("key %d: got %d, want %d", k, got[k], v)
				}
			}
		})
	}
}
