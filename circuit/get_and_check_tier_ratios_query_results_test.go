package circuit

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/constraint/solver"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/std/rangecheck"
)

type tierInput struct {
	Boundary *big.Int
	Ratio    uint64
}

type singleTierQueryCircuit struct {
	CAssets []CexAssetInfo

	AssetIndex      Variable
	UserCollateral  Variable
	CollateralIndex Variable
	CollateralFlag  Variable
	AssetPrice      Variable

	Expected    Variable
	CheckOutput bool
}

func (c singleTierQueryCircuit) Define(api API) error {
	r := rangecheck.New(api)
	for i := range c.CAssets {
		generateRapidArithmeticForCollateral(api, r, c.CAssets[i].LoanRatios)
	}
	t := constructLoanTierRatiosLookupTable(api, c.CAssets)

	tierRatiosLen := 3 * (len(c.CAssets[0].LoanRatios) + 1)
	maxTierIndex := len(c.CAssets[0].LoanRatios) - 1
	got := getAndCheckTierRatiosQueryResults(
		api, r, t,
		c.AssetIndex,
		c.UserCollateral,
		c.CollateralIndex,
		c.CollateralFlag,
		c.AssetPrice,
		tierRatiosLen,
		maxTierIndex,
	)
	if c.CheckOutput {
		api.AssertIsEqual(c.Expected, got)
	}
	return nil
}

type twoAssetOffsetIsolationCircuit struct {
	CAssets []CexAssetInfo

	UserCollateral  Variable
	CollateralIndex Variable
	CollateralFlag  Variable
	AssetPrice      Variable

	ExpectedAsset0 Variable
	ExpectedAsset1 Variable
}

func (c twoAssetOffsetIsolationCircuit) Define(api API) error {
	r := rangecheck.New(api)
	for i := range c.CAssets {
		generateRapidArithmeticForCollateral(api, r, c.CAssets[i].LoanRatios)
	}
	t := constructLoanTierRatiosLookupTable(api, c.CAssets)

	tierRatiosLen := 3 * (len(c.CAssets[0].LoanRatios) + 1)
	maxTierIndex := len(c.CAssets[0].LoanRatios) - 1

	got0 := getAndCheckTierRatiosQueryResults(
		api, r, t,
		0,
		c.UserCollateral,
		c.CollateralIndex,
		c.CollateralFlag,
		c.AssetPrice,
		tierRatiosLen,
		maxTierIndex,
	)
	got1 := getAndCheckTierRatiosQueryResults(
		api, r, t,
		1,
		c.UserCollateral,
		c.CollateralIndex,
		c.CollateralFlag,
		c.AssetPrice,
		tierRatiosLen,
		maxTierIndex,
	)

	api.AssertIsEqual(got0, c.ExpectedAsset0)
	api.AssertIsEqual(got1, c.ExpectedAsset1)
	return nil
}

func TestGetAndCheckTierRatiosQueryResultsEdgeCases(t *testing.T) {
	solver.RegisterHint(IntegerDivision)

	priceOne := big.NewInt(1)
	stdTiers := []tierInput{
		{Boundary: big.NewInt(100), Ratio: 100},
		{Boundary: big.NewInt(200), Ratio: 80},
		{Boundary: big.NewInt(300), Ratio: 50},
	}
	singleTier80 := []tierInput{
		{Boundary: big.NewInt(100), Ratio: 80},
	}
	floorTiers := []tierInput{
		{Boundary: big.NewInt(100), Ratio: 100},
		{Boundary: big.NewInt(200), Ratio: 33},
	}
	zeroRatioTiers := []tierInput{
		{Boundary: big.NewInt(100), Ratio: 100},
		{Boundary: big.NewInt(200), Ratio: 0},
	}
	zeroWidthTiers := []tierInput{
		{Boundary: big.NewInt(100), Ratio: 100},
		{Boundary: big.NewInt(100), Ratio: 80},
		{Boundary: big.NewInt(200), Ratio: 50},
	}

	maxTierPlusOne := new(big.Int).Add(new(big.Int).Set(utils.MaxTierBoundaryValue), big.NewInt(1))

	type tc struct {
		name       string
		tiers      []tierInput
		collateral *big.Int
		index      int
		flag       int
		price      *big.Int
		expectFail bool
	}

	tests := []tc{
		// P0: core + security boundaries
		{name: "first_tier_normal_range", tiers: stdTiers, collateral: big.NewInt(60), index: 0, flag: 0, price: priceOne},
		{name: "first_tier_equal_boundary", tiers: stdTiers, collateral: big.NewInt(100), index: 0, flag: 0, price: priceOne},
		{name: "middle_tier_normal_range", tiers: stdTiers, collateral: big.NewInt(150), index: 1, flag: 0, price: priceOne},
		{name: "middle_tier_equal_boundary", tiers: stdTiers, collateral: big.NewInt(200), index: 1, flag: 0, price: priceOne},
		{name: "last_tier_flag_zero", tiers: stdTiers, collateral: big.NewInt(250), index: 2, flag: 0, price: priceOne},
		{name: "flag_one_saturates_to_last_precomputed", tiers: stdTiers, collateral: big.NewInt(350), index: 2, flag: 1, price: priceOne},
		{name: "flag_one_with_equal_last_boundary_should_fail", tiers: stdTiers, collateral: big.NewInt(300), index: 2, flag: 1, price: priceOne, expectFail: true},
		{name: "flag_one_with_non_last_index_should_fail", tiers: stdTiers, collateral: big.NewInt(350), index: 1, flag: 1, price: priceOne, expectFail: true},
		{name: "index_greater_than_max_should_fail", tiers: stdTiers, collateral: big.NewInt(200), index: 3, flag: 0, price: priceOne, expectFail: true},
		{name: "flag_non_boolean_should_fail", tiers: stdTiers, collateral: big.NewInt(150), index: 1, flag: 2, price: priceOne, expectFail: true},
		{name: "zero_collateral_index_zero_should_pass", tiers: stdTiers, collateral: big.NewInt(0), index: 0, flag: 0, price: priceOne},
		{name: "zero_collateral_with_index_gt_zero_should_fail", tiers: stdTiers, collateral: big.NewInt(0), index: 1, flag: 1, price: priceOne, expectFail: true},
		{name: "index_too_low_for_value_should_fail", tiers: stdTiers, collateral: big.NewInt(250), index: 1, flag: 0, price: priceOne, expectFail: true},
		{name: "index_too_high_for_value_should_fail", tiers: stdTiers, collateral: big.NewInt(50), index: 2, flag: 0, price: priceOne, expectFail: true},
		{name: "flag_one_value_exceeds_max_tier_boundary_should_fail", tiers: stdTiers, collateral: maxTierPlusOne, index: 2, flag: 1, price: priceOne, expectFail: true},

		// P1: robustness
		{name: "single_tier_flag_zero", tiers: singleTier80, collateral: big.NewInt(70), index: 0, flag: 0, price: priceOne},
		{name: "single_tier_flag_one", tiers: singleTier80, collateral: big.NewInt(150), index: 0, flag: 1, price: priceOne},
		{name: "single_tier_flag_one_equal_boundary_should_fail", tiers: singleTier80, collateral: big.NewInt(100), index: 0, flag: 1, price: priceOne, expectFail: true},
		{name: "floor_semantics_non_divisible", tiers: floorTiers, collateral: big.NewInt(150), index: 1, flag: 0, price: priceOne},
		{name: "zero_ratio_tier_increment", tiers: zeroRatioTiers, collateral: big.NewInt(150), index: 1, flag: 0, price: priceOne},
		{name: "zero_width_tier_equal_boundary", tiers: zeroWidthTiers, collateral: big.NewInt(100), index: 0, flag: 0, price: priceOne},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			witnessAsset := makeLoanOnlyAsset(tt.tiers)
			template := singleTierQueryCircuit{
				CAssets:     make([]CexAssetInfo, 1),
				CheckOutput: !tt.expectFail,
			}
			template.CAssets[0] = blankLoanOnlyAsset(len(tt.tiers))

			cs, err := frontend.Compile(
				ecc.BN254.ScalarField(),
				r1cs.NewBuilder,
				&template,
				frontend.IgnoreUnconstrainedInputs(),
			)
			if err != nil {
				t.Fatalf("compile failed: %v", err)
			}

			witness := singleTierQueryCircuit{
				CAssets:         []CexAssetInfo{witnessAsset},
				AssetIndex:      0,
				UserCollateral:  tt.collateral,
				CollateralIndex: tt.index,
				CollateralFlag:  tt.flag,
				AssetPrice:      tt.price,
				Expected:        0,
				CheckOutput:     !tt.expectFail,
			}
			if !tt.expectFail {
				witness.Expected = expectedCollateralValue(tt.tiers, tt.collateral, tt.price, tt.index, tt.flag)
			}

			w, err := frontend.NewWitness(&witness, ecc.BN254.ScalarField())
			if err != nil {
				t.Fatalf("new witness failed: %v", err)
			}

			err = cs.IsSolved(w)
			if tt.expectFail {
				if err == nil {
					t.Fatalf("expected failure, but witness passed")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected success, got error: %v", err)
			}
		})
	}
}

func TestGetAndCheckTierRatiosQueryResultsMultiAssetOffsetIsolation(t *testing.T) {
	solver.RegisterHint(IntegerDivision)

	asset0Tiers := []tierInput{
		{Boundary: big.NewInt(100), Ratio: 100},
		{Boundary: big.NewInt(200), Ratio: 100},
	}
	asset1Tiers := []tierInput{
		{Boundary: big.NewInt(100), Ratio: 50},
		{Boundary: big.NewInt(200), Ratio: 50},
	}

	template := twoAssetOffsetIsolationCircuit{
		CAssets: []CexAssetInfo{
			blankLoanOnlyAsset(len(asset0Tiers)),
			blankLoanOnlyAsset(len(asset1Tiers)),
		},
	}
	cs, err := frontend.Compile(
		ecc.BN254.ScalarField(),
		r1cs.NewBuilder,
		&template,
		frontend.IgnoreUnconstrainedInputs(),
	)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	collateral := big.NewInt(150)
	price := big.NewInt(1)
	index := 1
	flag := 0
	expected0 := expectedCollateralValue(asset0Tiers, collateral, price, index, flag) // 150
	expected1 := expectedCollateralValue(asset1Tiers, collateral, price, index, flag) // 75

	witness := twoAssetOffsetIsolationCircuit{
		CAssets: []CexAssetInfo{
			makeLoanOnlyAsset(asset0Tiers),
			makeLoanOnlyAsset(asset1Tiers),
		},
		UserCollateral:  collateral,
		CollateralIndex: index,
		CollateralFlag:  flag,
		AssetPrice:      price,
		ExpectedAsset0:  expected0,
		ExpectedAsset1:  expected1,
	}

	w, err := frontend.NewWitness(&witness, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatalf("new witness failed: %v", err)
	}
	if err := cs.IsSolved(w); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

func makeLoanOnlyAsset(tiers []tierInput) CexAssetInfo {
	asset := blankLoanOnlyAsset(len(tiers))
	asset.BasePrice = big.NewInt(1)
	for i, tr := range tiers {
		asset.LoanRatios[i] = TierRatio{
			BoundaryValue:    new(big.Int).Set(tr.Boundary),
			Ratio:            tr.Ratio,
			PrecomputedValue: 0,
		}
	}
	return asset
}

func blankLoanOnlyAsset(tierCount int) CexAssetInfo {
	asset := CexAssetInfo{
		TotalEquity:               0,
		TotalDebt:                 0,
		BasePrice:                 0,
		LoanCollateral:            0,
		MarginCollateral:          0,
		PortfolioMarginCollateral: 0,
		LoanRatios:                make([]TierRatio, tierCount),
		MarginRatios:              make([]TierRatio, tierCount),
		PortfolioMarginRatios:     make([]TierRatio, tierCount),
	}
	for i := 0; i < tierCount; i++ {
		asset.LoanRatios[i] = TierRatio{
			BoundaryValue:    0,
			Ratio:            0,
			PrecomputedValue: 0,
		}
		asset.MarginRatios[i] = TierRatio{
			BoundaryValue:    0,
			Ratio:            0,
			PrecomputedValue: 0,
		}
		asset.PortfolioMarginRatios[i] = TierRatio{
			BoundaryValue:    0,
			Ratio:            0,
			PrecomputedValue: 0,
		}
	}
	return asset
}

func expectedCollateralValue(tiers []tierInput, collateral, price *big.Int, index, flag int) *big.Int {
	if len(tiers) == 0 {
		panic("tiers must not be empty")
	}
	if index < 0 || index >= len(tiers) {
		panic(fmt.Sprintf("invalid index %d for tier length %d", index, len(tiers)))
	}

	precomputed := make([]*big.Int, len(tiers))
	hundred := big.NewInt(100)

	precomputed[0] = new(big.Int).Mul(tiers[0].Boundary, new(big.Int).SetUint64(tiers[0].Ratio))
	precomputed[0].Quo(precomputed[0], hundred)
	for i := 1; i < len(tiers); i++ {
		diffBoundary := new(big.Int).Sub(tiers[i].Boundary, tiers[i-1].Boundary)
		step := new(big.Int).Mul(diffBoundary, new(big.Int).SetUint64(tiers[i].Ratio))
		step.Quo(step, hundred)
		precomputed[i] = new(big.Int).Add(precomputed[i-1], step)
	}

	if flag == 1 {
		return new(big.Int).Set(precomputed[len(precomputed)-1])
	}

	collateralValue := new(big.Int).Mul(new(big.Int).Set(collateral), price)
	lowerBoundary := big.NewInt(0)
	lowerPrecomputed := big.NewInt(0)
	if index > 0 {
		lowerBoundary = tiers[index-1].Boundary
		lowerPrecomputed = precomputed[index-1]
	}

	diff := new(big.Int).Sub(collateralValue, lowerBoundary)
	diff.Mul(diff, new(big.Int).SetUint64(tiers[index].Ratio))
	diff.Quo(diff, hundred)

	return new(big.Int).Add(new(big.Int).Set(lowerPrecomputed), diff)
}
