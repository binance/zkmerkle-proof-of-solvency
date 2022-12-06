package config

import (
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"math/big"
)

type Config struct {
	ProofTable    string
	ZkKeyName     string
	CexAssetsInfo []utils.CexAssetInfo
}

type UserConfig struct {
	AccountIndex  uint32
	AccountIdHash string
	TotalEquity   big.Int
	TotalDebt     big.Int
	Root          string
	Assets        []utils.AccountAsset
	Proof         []string
}
