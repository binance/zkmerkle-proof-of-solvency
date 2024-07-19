package utils

import (
	"math/big"
	"sort"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

const (
	// BatchCreateUserOpsCounts = 864
	AccountTreeDepth         = 28
	AssetCounts              = 350
	// TierCount: must be even number, the cex assets commitment will depend on the TierCount/2 parts
	TierCount				 = 12
	RedisLockKey             = "prover_mutex_key"
	R1csBatchSize            = 1000000
)

var (
	ZeroBigInt                    = new(big.Int).SetInt64(0)
	OneBigInt                     = new(big.Int).SetInt64(1)
	PercentageMultiplier          = new(big.Int).SetUint64(100)
	MaxTierBoundaryValue, _       = new(big.Int).SetString("332306998946228968225951765070086144", 10) // (pow(2,118))
	Uint64MaxValueBigInt, _       = new(big.Int).SetString("18446744073709551616", 10)
	Uint64MaxValueBigIntSquare, _ = new(big.Int).SetString("340282366920938463463374607431768211456", 10)
	Uint8MaxValueBigInt, _        = new(big.Int).SetString("256", 10)
	Uint16MaxValueBigInt, _       = new(big.Int).SetString("65536", 10)
	Uint126MaxValueBigInt, _      = new(big.Int).SetString("85070591730234615865843651857942052864", 10)
	Uint134MaxValueBigInt, _      = new(big.Int).SetString("21778071482940061661655974875633165533184", 10)
	Uint64MaxValueFr              = new(fr.Element).SetBigInt(Uint64MaxValueBigInt)
	Uint64MaxValueFrSquare        = new(fr.Element).SetBigInt(Uint64MaxValueBigIntSquare)
	Uint8MaxValueFr               = new(fr.Element).SetBigInt(Uint8MaxValueBigInt)
	Uint16MaxValueFr 			  = new(fr.Element).SetBigInt(Uint16MaxValueBigInt)
	Uint126MaxValueFr             = new(fr.Element).SetBigInt(Uint126MaxValueBigInt)
	Uint134MaxValueFr             = new(fr.Element).SetBigInt(Uint134MaxValueBigInt)
	MaxTierBoundaryValueFr		  = new(fr.Element).SetBigInt(MaxTierBoundaryValue)
	PercentageMultiplierFr     	  = new(fr.Element).SetBigInt(PercentageMultiplier)

	AssetTypeForTwoDigits         = map[string]bool{
		"BTTC":  true,
		"bttc":  true,
		"SHIB":  true,
		"shib":  true,
		"LUNC":  true,
		"lunc":  true,
		"XEC":   true,
		"xec":   true,
		"WIN":   true,
		"win":   true,
		"BIDR":  true,
		"bidr":  true,
		"SPELL": true,
		"spell": true,
		"HOT":   true,
		"hot":   true,
		"DOGE":  true,
		"doge":  true,
        "PEPE":  true,
		"pepe":  true,
	}
	// the key is the number of assets user own
	// the value is the number of batch create user ops
	BatchCreateUserOpsCountsTiers = map[int]int {
		350: 128,
		50: 580,
	}
	AssetCountsTiers = make([]int, 0)

	// one Fr element is 252 bits, it contains 16 16-bit elements at most
	PowersOfSixteenBits           [15]fr.Element
)

func init() {
	initValue := new(big.Int).SetUint64(1)
	for i := 0; i < 15; i++ {
		PowersOfSixteenBits[i].SetBigInt(initValue)
		initValue.Mul(initValue, big.NewInt(65536))
	}
	for k := range BatchCreateUserOpsCountsTiers {
		AssetCountsTiers = append(AssetCountsTiers, k)
	}
	sort.Ints(AssetCountsTiers)
}
