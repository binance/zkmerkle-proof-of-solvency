package utils

import (
	"fmt"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
	"gorm.io/hints"
)

const (
	// BatchCreateUserOpsCounts = 864
	AccountTreeDepth         = 28
	AssetCounts              = 500
	// TierCount: must be even number, the cex assets commitment will depend on the TierCount/2 parts
	TierCount				 = 12
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
		"FLOKI": true,
		"floki": true,
		"IDRT":  true,
		"idrt":  true,
		"DOGS":  true,
		"dogs":  true,
		"BONK":  true,
		"bonk":  true,
		"1000SATS": true,
		"1000sats": true,
        "NEIRO": true,
		"neiro": true,
		"1000PEPPER" : true,
		"1000pepper" : true,
		"NOT": true,
		"not": true,
		"NFT": true,
		"nft": true,
		"BOME": true,
		"bome": true,
		"1MBABYDOGE": true,
		"1mbabydoge": true,
		"HMSTR": true,
		"hmstr": true,
		"WLFI": true,
		"wlfi": true,
		"PUMP": true,
		"pump": true,
		"MONKY": true,
		"monky": true,
		"1000CHEEMS": true,
		"1000cheems": true,
		"IDR": true,
		"idr": true,
	}
	// the key is the number of assets user own
	// the value is the number of batch create user ops
	BatchCreateUserOpsCountsTiers = map[int]int {
		500: 92,
		50: 760,
	}
	AssetCountsTiers = make([]int, 0)

	// one Fr element is 252 bits, it contains 16 16-bit elements at most
	PowersOfSixteenBits           [15]fr.Element
	MaxExecutionTimeHint         = hints.New("MAX_EXECUTION_TIME(10000)")
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

	zero := &fr.Element{0, 0, 0, 0}
	tempHash := poseidon.Poseidon(zero, zero, zero, zero, zero).Bytes()
	NilAccountHash = tempHash[:]
	// fmt.Printf("NilAccountHash: %x\n", NilAccountHash)

	if testTiers := strings.TrimSpace(os.Getenv("ZKPOR_TEST_TIERS")); testTiers != "" {
		parsed, err := parseTiers(testTiers)
		if err != nil {
			panic("failed to parse ZKPOR_TEST_TIERS: " + err.Error())
		}
		BatchCreateUserOpsCountsTiers = parsed
		AssetCountsTiers = make([]int, 0)
		for k := range BatchCreateUserOpsCountsTiers {
			AssetCountsTiers = append(AssetCountsTiers, k)
		}
		sort.Ints(AssetCountsTiers)
		fmt.Printf("ZKPOR_TEST_TIERS override active: %v\n", BatchCreateUserOpsCountsTiers)
	}
}

// parseTiers parses a tier string like "500:4,50:20" into a map[int]int.
func parseTiers(s string) (map[int]int, error) {
	result := make(map[int]int)
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tier pair: %q", pair)
		}
		k, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid asset count %q: %w", parts[0], err)
		}
		v, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid ops count %q: %w", parts[1], err)
		}
		result[k] = v
	}
	return result, nil
}
