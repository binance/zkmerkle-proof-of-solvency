package config

import (
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"math/big"
)

type Config struct {
	ProofTable    string
	ZkKeyName     []string
	AssetsCountTiers []int
	CexAssetsInfo []utils.CexAssetInfo
}

type UserConfig struct {
	AccountIndex  uint32
	AccountIdHash string
	TotalEquity   big.Int
	TotalDebt     big.Int
	TotalCollateral big.Int
	Root          string
	Assets        []utils.AccountAsset
	Proof         []string
}
