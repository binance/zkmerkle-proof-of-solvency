package circuit

import (
	"math/big"

	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash/poseidon"
	"github.com/consensys/gnark/std/lookup/logderivlookup"
)

func verifyMerkleProof(api API, merkleRoot Variable, node Variable, proofSet, helper []Variable) {
	for i := 0; i < len(proofSet); i++ {
		api.AssertIsBoolean(helper[i])
		d1 := api.Select(helper[i], proofSet[i], node)
		d2 := api.Select(helper[i], node, proofSet[i])
		node = poseidon.Poseidon(api, d1, d2)
	}
	// Compare our calculated Merkle root to the desired Merkle root.
	api.AssertIsEqual(merkleRoot, node)
}

func updateMerkleProof(api API, node Variable, proofSet, helper []Variable) (root Variable) {
	for i := 0; i < len(proofSet); i++ {
		api.AssertIsBoolean(helper[i])
		d1 := api.Select(helper[i], proofSet[i], node)
		d2 := api.Select(helper[i], node, proofSet[i])
		node = poseidon.Poseidon(api, d1, d2)
	}
	root = node
	return root
}

func accountIdToMerkleHelper(api API, accountId Variable) []Variable {
	merkleHelpers := api.ToBinary(accountId, utils.AccountTreeDepth)
	return merkleHelpers
}

func computeUserAssetsCommitment(api API, flattenAssets []Variable) Variable {
	nEles := (len(flattenAssets) + 2) / 3
	quotientEles := len(flattenAssets) / 3
	remainderEles := len(flattenAssets) % 3
	tmpUserAssets := make([]Variable, nEles)
	for i := 0; i < quotientEles; i++ {
		tmpUserAssets[i] = api.Add(api.Mul(flattenAssets[3*i], utils.Uint64MaxValueFrSquare),
			api.Mul(flattenAssets[3*i+1], utils.Uint64MaxValueFr), flattenAssets[3*i+2])
	}
	var lastEle Variable = 0
	for i := 0; i < remainderEles; i++ {
		lastEle = api.Add(api.Mul(lastEle, utils.Uint64MaxValueFr), flattenAssets[3*quotientEles+i])
	}
	for i := remainderEles; i < 3; i++ {
		lastEle = api.Mul(lastEle, utils.Uint64MaxValueFr)
	}
	commitment := poseidon.Poseidon(api, tmpUserAssets...)
	return commitment
}

// one variable: TotalEquity + TotalDebt + BasePrice
// one variable: LoanCollateral + MarginCollateral + PortfolioMarginCollateral
// one variable contain two TierRatios and the length of TierRatios is even
func getVariableCountOfCexAsset(cexAsset CexAssetInfo) int {
	res := 2
	res += len(cexAsset.LoanRatios) / 2
	res += len(cexAsset.MarginRatios) / 2
	res += len(cexAsset.PortfolioMarginRatios) / 2
	return res
}

func convertTierRatiosToVariables(api API, ratios []TierRatio, res []Variable) {
	for i := 0; i < len(ratios); i += 2 {
		v := api.Add(ratios[i].Ratio, api.Mul(ratios[i].BoundaryValue, utils.Uint8MaxValueFr))
		v1 := api.Add(api.Mul(ratios[i+1].Ratio, utils.Uint126MaxValueFr), api.Mul(ratios[i+1].BoundaryValue, utils.Uint134MaxValueFr))
		res[i/2] = api.Add(v, v1)
	}
}

func fillCexAssetCommitment(api API, asset CexAssetInfo, currentIndex int, commitments []Variable) {
	counts := getVariableCountOfCexAsset(asset)
	commitments[currentIndex*counts] = api.Add(api.Mul(asset.TotalEquity, utils.Uint64MaxValueFrSquare),
		api.Mul(asset.TotalDebt, utils.Uint64MaxValueFr), asset.BasePrice)

	commitments[currentIndex*counts+1] = api.Add(api.Mul(asset.LoanCollateral, utils.Uint64MaxValueFrSquare),
		api.Mul(asset.MarginCollateral, utils.Uint64MaxValueFr), asset.PortfolioMarginCollateral)

	convertTierRatiosToVariables(api, asset.LoanRatios, commitments[currentIndex*counts+2:])
	convertTierRatiosToVariables(api, asset.MarginRatios, commitments[currentIndex*counts+2+len(asset.LoanRatios)/2:])
	convertTierRatiosToVariables(api, asset.PortfolioMarginRatios, commitments[currentIndex*counts+2+len(asset.LoanRatios)/2+len(asset.MarginRatios)/2:])
}

func generateRapidArithmeticForCollateral(api API, r frontend.Rangechecker, tierRatios []TierRatio) {
	tierRatios[0].PrecomputedValue = checkAndGetIntegerDivisionRes(api, r, api.Mul(tierRatios[0].BoundaryValue, tierRatios[0].Ratio))
	api.AssertIsLessOrEqualNOp(tierRatios[0].Ratio, utils.PercentageMultiplierFr, 8, true)
	api.AssertIsLessOrEqualNOp(tierRatios[0].BoundaryValue, utils.MaxTierBoundaryValueFr, 128, true)
	for i := 1; i < len(tierRatios); i++ {
		api.AssertIsLessOrEqualNOp(tierRatios[i-1].BoundaryValue, tierRatios[i].BoundaryValue, 128, true)
		api.AssertIsLessOrEqualNOp(tierRatios[i].Ratio, utils.PercentageMultiplierFr, 8, true)
		api.AssertIsLessOrEqualNOp(tierRatios[i].BoundaryValue, utils.MaxTierBoundaryValueFr, 128, true)
		diffBoundary := api.Sub(tierRatios[i].BoundaryValue, tierRatios[i-1].BoundaryValue)
		current := checkAndGetIntegerDivisionRes(api, r, api.Mul(diffBoundary, tierRatios[i].Ratio))
		tierRatios[i].PrecomputedValue = api.Add(tierRatios[i-1].PrecomputedValue, current)
	}

	for i := 0; i < len(tierRatios); i++ {
		r.Check(tierRatios[i].PrecomputedValue, 128)
		r.Check(tierRatios[i].Ratio, 8)
		r.Check(tierRatios[i].BoundaryValue, 128)
	}
}

func IntegerDivision(_ *big.Int, in []*big.Int, out []*big.Int) error {
	// in[0] is the dividend
	// in[1] is the divisor
	// out[0] is the quotient
	// out[1] is the remainder
	out[0].DivMod(in[0], in[1], out[1])
	return nil
}

func getAndCheckTierRatiosQueryResults(api API, r frontend.Rangechecker, tierRatiosTable *logderivlookup.Table,
	assetIndex, userCollateral, collateralIndex, collateralFlag, assetPrice, collateralTierRatiosLen Variable) (collateralValueRes Variable) {
	// All indexes are shifted by 1 overall because we add a dummy tier ratio at the beginning
	// 18 = 3 * 6: 3 means the number of collateral types, 6 means the number of tier ratios queries for each collateral type
	numOfTierRatioFields := 3
	queries := make([]Variable, 6)
	gap := api.Mul(assetIndex, collateralTierRatiosLen)
	for i := 0; i < 2; i++ {
		startPosition := api.Mul(collateralIndex, 3)
		queries[i*numOfTierRatioFields+0] = api.Add(startPosition, gap)
		queries[i*numOfTierRatioFields+1] = api.Add(startPosition, api.Add(gap, 1))
		queries[i*numOfTierRatioFields+2] = api.Add(startPosition, api.Add(gap, 2))
		collateralIndex = api.Add(collateralIndex, 1)
	}
	results := tierRatiosTable.Lookup(queries...)
	collateralValue := api.Mul(userCollateral, assetPrice)
	// results[0] is less than 2^128 which is constrainted in the GenerateRapidArithmeticForCollateral
	cr := api.CmpNOp(collateralValue, results[0], 128, true)
	// cr only can be 0 or 1
	// cr is 0 in the special case that userAssets.LoanCollateral is 0;
	api.AssertIsEqual(cr, api.Select(api.IsZero(collateralValue), 0, 1))
	// results[3] is the upper boundary value
	upperBoundaryValue := api.Select(api.IsZero(collateralFlag), results[3], utils.MaxTierBoundaryValueFr)
	api.AssertIsLessOrEqualNOp(collateralValue, upperBoundaryValue, 128, true)
	// results[4] is ratio of upper boundary value
	// diffValue = (collateralValue - lower boundary value) * ratio
	diffValue := api.Mul(api.Sub(collateralValue, results[0]), results[4])
	quotient := checkAndGetIntegerDivisionRes(api, r, diffValue)
	// Check diffValue is
	// results[2] is the precomputed value of lower boundary value
	collateralValueRes = api.Select(api.IsZero(collateralFlag), api.Add(results[2], quotient), results[5])
	return collateralValueRes
}

func checkAndGetIntegerDivisionRes(api API, r frontend.Rangechecker, dividend Variable) (quotient Variable) {
	quotientRes, err := api.NewHint(IntegerDivision, 2, dividend, utils.PercentageMultiplierFr)
	if err != nil {
		panic(err)
	}
	r.Check(quotientRes[0], 128)
	r.Check(quotientRes[1], 8)
	api.AssertIsLessOrEqualNOp(quotientRes[1], utils.PercentageMultiplierFr, 8, true)
	api.AssertIsEqual(api.Add(api.Mul(quotientRes[0], utils.PercentageMultiplierFr), quotientRes[1]), dividend)
	return quotientRes[0]
}

func constructLoanTierRatiosLookupTable(api API, cexAssetInfo []CexAssetInfo) *logderivlookup.Table {
	t := logderivlookup.New(api)
	for i := 0; i < len(cexAssetInfo); i++ {
		// dummy tier ratio
		for i := 0; i < 3; i++ {
			t.Insert(0)
		}
		for j := 0; j < len(cexAssetInfo[i].LoanRatios); j++ {
			t.Insert(cexAssetInfo[i].LoanRatios[j].BoundaryValue)
			t.Insert(cexAssetInfo[i].LoanRatios[j].Ratio)
			t.Insert(cexAssetInfo[i].LoanRatios[j].PrecomputedValue)
		}
	}
	return t
}

func constructMarginTierRatiosLookupTable(api API, cexAssetInfo []CexAssetInfo) *logderivlookup.Table {
	t := logderivlookup.New(api)
	for i := 0; i < len(cexAssetInfo); i++ {
		// dummy tier ratio
		for i := 0; i < 3; i++ {
			t.Insert(0)
		}
		for j := 0; j < len(cexAssetInfo[i].MarginRatios); j++ {
			t.Insert(cexAssetInfo[i].MarginRatios[j].BoundaryValue)
			t.Insert(cexAssetInfo[i].MarginRatios[j].Ratio)
			t.Insert(cexAssetInfo[i].MarginRatios[j].PrecomputedValue)
		}
	}
	return t
}

func constructPortfolioTierRatiosLookupTable(api API, cexAssetInfo []CexAssetInfo) *logderivlookup.Table {
	t := logderivlookup.New(api)
	for i := 0; i < len(cexAssetInfo); i++ {
		// dummy tier ratio
		for i := 0; i < 3; i++ {
			t.Insert(0)
		}
		for j := 0; j < len(cexAssetInfo[i].PortfolioMarginRatios); j++ {
			t.Insert(cexAssetInfo[i].PortfolioMarginRatios[j].BoundaryValue)
			t.Insert(cexAssetInfo[i].PortfolioMarginRatios[j].Ratio)
			t.Insert(cexAssetInfo[i].PortfolioMarginRatios[j].PrecomputedValue)
		}
	}
	return t
}

func calcAndSetCollateralInfo(assetIndex int, ua *UserAssetInfo, um *utils.AccountAsset, cexInfo []utils.CexAssetInfo) {
	p := cexInfo[assetIndex]
	assestPrice := new(big.Int).SetUint64(p.BasePrice)
	userLoanCollateral := new(big.Int).SetUint64(um.Loan)
	userLoanCollateral.Mul(userLoanCollateral, assestPrice)
	userMarginCollateral := new(big.Int).SetUint64(um.Margin)
	userMarginCollateral.Mul(userMarginCollateral, assestPrice)
	userPortfolioMarginCollateral := new(big.Int).SetUint64(um.PortfolioMargin)
	userPortfolioMarginCollateral.Mul(userPortfolioMarginCollateral, assestPrice)

	var findFlag bool = false
	for i := 0; i < len(p.LoanRatios); i++ {
		if userLoanCollateral.Cmp(p.LoanRatios[i].BoundaryValue) <= 0 {
			ua.LoanCollateralIndex = i
			ua.LoanCollateralFlag = 0
			findFlag = true
			break
		}
	}
	if !findFlag {
		ua.LoanCollateralIndex = len(p.LoanRatios) - 1
		ua.LoanCollateralFlag = 1
	}

	findFlag = false
	for i := 0; i < len(p.MarginRatios); i++ {
		if userMarginCollateral.Cmp(p.MarginRatios[i].BoundaryValue) <= 0 {
			ua.MarginCollateralIndex = i
			ua.MarginCollateralFlag = 0
			findFlag = true
			break
		}
	}
	if !findFlag {
		ua.MarginCollateralIndex = len(p.MarginRatios) - 1
		ua.MarginCollateralFlag = 1
	}

	findFlag = false
	for i := 0; i < len(p.PortfolioMarginRatios); i++ {
		if userPortfolioMarginCollateral.Cmp(p.PortfolioMarginRatios[i].BoundaryValue) <= 0 {
			ua.PortfolioMarginCollateralIndex = i
			ua.PortfolioMarginCollateralFlag = 0
			findFlag = true
			break
		}
	}
	if !findFlag {
		ua.PortfolioMarginCollateralIndex = len(p.PortfolioMarginRatios) - 1
		ua.PortfolioMarginCollateralFlag = 1
	}
}
