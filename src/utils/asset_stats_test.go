package utils

import (
	"reflect"
	"testing"
)

func assetsWith(nonEmpty int, empty int) []AccountAsset {
	assets := make([]AccountAsset, 0, nonEmpty+empty)
	for i := 0; i < nonEmpty; i++ {
		assets = append(assets, AccountAsset{Index: uint16(i), Equity: 1})
	}
	for i := 0; i < empty; i++ {
		assets = append(assets, AccountAsset{Index: uint16(nonEmpty + i)})
	}
	return assets
}

func TestCountNonEmptyAssets(t *testing.T) {
	if got := CountNonEmptyAssets(assetsWith(3, 5)); got != 3 {
		t.Fatalf("want 3, got %d", got)
	}
	if got := CountNonEmptyAssets(assetsWith(0, 4)); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
	if got := CountNonEmptyAssets(nil); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
}

func TestCountUsersByAssetCount(t *testing.T) {
	accounts := map[int][]AccountInfo{
		50: {
			{Assets: assetsWith(1, 2)},
			{Assets: assetsWith(1, 0)},
			{Assets: assetsWith(3, 10)},
		},
		500: {
			{Assets: assetsWith(60, 5)},
			{Assets: assetsWith(0, 3)},
		},
	}
	want := map[int]int{0: 1, 1: 2, 3: 1, 60: 1}
	if got := CountUsersByAssetCount(accounts); !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	if got := CountUsersByAssetCount(nil); len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}
