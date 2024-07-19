package circuit

import (
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash/poseidon"
	"github.com/consensys/gnark/std/lookup/logderivlookup"
	"math/big"
)

func VerifyMerkleProof(api API, merkleRoot Variable, node Variable, proofSet, helper []Variable) {
	for i := 0; i < len(proofSet); i++ {
		api.AssertIsBoolean(helper[i])
		d1 := api.Select(helper[i], proofSet[i], node)
		d2 := api.Select(helper[i], node, proofSet[i])
		node = poseidon.Poseidon(api, d1, d2)
	}
	// Compare our calculated Merkle root to the desired Merkle root.
	api.AssertIsEqual(merkleRoot, node)
}

func UpdateMerkleProof(api API, node Variable, proofSet, helper []Variable) (root Variable) {
	for i := 0; i < len(proofSet); i++ {
		api.AssertIsBoolean(helper[i])
		d1 := api.Select(helper[i], proofSet[i], node)
		d2 := api.Select(helper[i], node, proofSet[i])
		node = poseidon.Poseidon(api, d1, d2)
	}
	root = node
	return root
}

func AccountIdToMerkleHelper(api API, accountId Variable) []Variable {
	merkleHelpers := api.ToBinary(accountId, utils.AccountTreeDepth)
	return merkleHelpers
}

// check value is in [0, 2^64-1] range
func CheckValueInRange(api API, value Variable) {
	api.ToBinary(value, 64)
}

func ComputeUserAssetsCommitment(api API, assets []UserAssetMeta) Variable {
	nEles := (len(assets)*5 + 2) / 3
	tmpUserAssets := make([]Variable, nEles)
	flattenAssets := make([]Variable, nEles*3)
	for i := 0; i < len(assets); i++ {
		flattenAssets[5*i] = assets[i].Equity
		flattenAssets[5*i+1] = assets[i].Debt
		flattenAssets[5*i+2] = assets[i].VipLoanCollateral
		flattenAssets[5*i+3] = assets[i].MarginCollateral
		flattenAssets[5*i+4] = assets[i].PortfolioMarginCollateral
	}
	for i := len(assets) * 5; i < len(flattenAssets); i++ {
		flattenAssets[i] = 0
	}
	for i := 0; i < len(tmpUserAssets); i++ {
		tmpUserAssets[i] = api.Add(api.Mul(flattenAssets[3*i], utils.Uint64MaxValueFrSquare),
			api.Mul(flattenAssets[3*i+1], utils.Uint64MaxValueFr), flattenAssets[3*i+2])
	}
	commitment := poseidon.Poseidon(api, tmpUserAssets...)
	return commitment
}

// one variable: TotalEquity + TotalDebt + BasePrice
// one variable: VipLoanCollateral + MarginCollateral + PortfolioMarginCollateral
// one variable contain two TierRatios and the length of TierRatios is even
func GetVariableCountOfCexAsset(cexAsset CexAssetInfo) int {
	res := 2 
	res += len(cexAsset.VipLoanRatios) / 2
	res += len(cexAsset.MarginRatios) / 2
	res += len(cexAsset.PortfolioMarginRatios) / 2
	return res
}

func ConvertTierRatiosToVariables(api API, ratios []TierRatio, res []Variable) {
	for i := 0; i < len(ratios); i += 2 {
		v := api.Add(ratios[i].Ratio, api.Mul(ratios[i].BoundaryValue, utils.Uint8MaxValueFr))
		v1 := api.Add(api.Mul(ratios[i+1].Ratio, utils.Uint126MaxValueFr), api.Mul(ratios[i+1].BoundaryValue, utils.Uint134MaxValueFr))
		res[i/2] = api.Add(v, v1)
	}
}

func FillCexAssetCommitment(api API, asset CexAssetInfo, currentIndex int, commitments []Variable) {
	counts := GetVariableCountOfCexAsset(asset)
	commitments[currentIndex*counts] = api.Add(api.Mul(asset.TotalEquity, utils.Uint64MaxValueFrSquare),
			api.Mul(asset.TotalDebt, utils.Uint64MaxValueFr), asset.BasePrice)
	
	commitments[currentIndex*counts+1] = api.Add(api.Mul(asset.VipLoanCollateral, utils.Uint64MaxValueFrSquare),
			api.Mul(asset.MarginCollateral, utils.Uint64MaxValueFr), asset.PortfolioMarginCollateral)

	ConvertTierRatiosToVariables(api, asset.VipLoanRatios, commitments[currentIndex*counts+2:])
	ConvertTierRatiosToVariables(api, asset.MarginRatios, commitments[currentIndex*counts+2+len(asset.VipLoanRatios)/2:])
	ConvertTierRatiosToVariables(api, asset.PortfolioMarginRatios, commitments[currentIndex*counts+2+len(asset.VipLoanRatios)/2+len(asset.MarginRatios)/2:])
}

func GenerateRapidArithmeticForCollateral(api API, r frontend.Rangechecker, tierRatios []TierRatio) {
	tierRatios[0].PrecomputedValue = CheckAndGetIntegerDivisionRes(api, r, api.Mul(tierRatios[0].BoundaryValue, tierRatios[0].Ratio))
	for i := 1; i < len(tierRatios); i++ {
		api.AssertIsLessOrEqualNOp(tierRatios[i-1].BoundaryValue, tierRatios[i].BoundaryValue, 128, true)
		api.AssertIsLessOrEqualNOp(tierRatios[i].Ratio, utils.PercentageMultiplierFr, 8, true)
		api.AssertIsLessOrEqualNOp(tierRatios[i].BoundaryValue, utils.MaxTierBoundaryValueFr, 128, true)
		diffBoundary := api.Sub(tierRatios[i].BoundaryValue, tierRatios[i-1].BoundaryValue)
		current := CheckAndGetIntegerDivisionRes(api, r, api.Mul(diffBoundary, tierRatios[i].Ratio))
		tierRatios[i].PrecomputedValue = api.Add(tierRatios[i-1].PrecomputedValue, current)
	}

	for i := 0; i < len(tierRatios); i++ {
		r.Check(tierRatios[i].PrecomputedValue, 128)
		r.Check(tierRatios[i].Ratio, 8)
		r.Check(tierRatios[i].BoundaryValue, 128)
		// TODO
		// CheckAndGetIntegerDivisionRes(api, r, tierRatios[i].BoundaryValue)
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

func ComputeCollateral(api API, collateral Variable, tierRatios []TierRatio) Variable {
	var res Variable
	var firstFlag Variable = 0

	compareRes := api.CmpNOp(tierRatios[0].BoundaryValue, collateral, 128, true)
	flag := api.IsZero(api.Sub(compareRes, 1))
	firstFlag = api.Xor(firstFlag, flag)
	res = api.Select(firstFlag, api.Mul(collateral, tierRatios[0].Ratio), 0)
	
	for i := 1; i < len(tierRatios); i++ {
		compareRes = api.CmpNOp(tierRatios[i].BoundaryValue, collateral, 128, true)
		// flag is true if tierRatios[i].BoundaryValue > collateral
		// flag is false if tierRatios[i].BoundaryValue <= collateral
		flag = api.IsZero(api.Sub(compareRes, 1))
		// Only the first time that boundary value is greater than collateral, 
		// the firstFlag to 1. Otherwise, the firstFlag is 0.
		firstFlag = api.Xor(firstFlag, flag)
		
		v := api.Select(firstFlag, tierRatios[i-1].PrecomputedValue, 0)
		res = api.Add(res, v)

		v1 := api.Select(firstFlag, api.Sub(collateral, tierRatios[i-1].BoundaryValue), 0)
		v1 = api.Div(api.Mul(v1, tierRatios[i].Ratio), utils.PercentageMultiplierFr)
		res = api.Add(res, v1)
		firstFlag = api.Select(flag, 1, 0)
	}
	// The last tier boundary value is less or equal than collateral
	res = api.Select(firstFlag, res, tierRatios[len(tierRatios)-1].PrecomputedValue)
	return res
}

func GetAndCheckTierRatiosQueryResults(api API, r frontend.Rangechecker, tierRatiosTable *logderivlookup.Table, userAssets UserAssetInfo, 
									userVipLoanCollateral, userMarginCollateral, userPortfolioMarginCollateral Variable,
									assetPrice Variable, vipLoanTierRatiosLen, marginTierRatiosLen, portfolioMarginTierRatiosLen int) (collateralValueRes [3]Variable) {
	
	// All indexes are shifted by 1 overall because we add a dummy tier ratio at the beginning
	// 18 = 3 * 6: 3 means the number of collateral types, 6 means the number of tier ratios queires for each collateral type
	numOfTierRatioFields := 3
	queries := make([]Variable, 18)
	gap := api.Mul(userAssets.AssetIndex, (vipLoanTierRatiosLen + marginTierRatiosLen + portfolioMarginTierRatiosLen))
	
	vipLoanCollateralIndex := userAssets.VipLoanCollateralIndex
	for p := 0; p < 2; p++ {
		startPosition := api.Mul(vipLoanCollateralIndex, 3)
		queries[p*numOfTierRatioFields+0] = api.Add(startPosition, gap)
		queries[p*numOfTierRatioFields+1] = api.Add(startPosition, api.Add(gap, 1))
		queries[p*numOfTierRatioFields+2] = api.Add(startPosition, api.Add(gap, 2))
		vipLoanCollateralIndex = api.Add(vipLoanCollateralIndex, 1)
	}

	gap = api.Add(gap, vipLoanTierRatiosLen)
	marginCollateralIndex := userAssets.MarginCollateralIndex
	for p := 0; p < 2; p++ {
		startPosition := api.Mul(marginCollateralIndex, 3)
		queries[p*numOfTierRatioFields+0+6] = api.Add(startPosition, gap)
		queries[p*numOfTierRatioFields+1+6] = api.Add(startPosition, api.Add(gap, 1))
		queries[p*numOfTierRatioFields+2+6] = api.Add(startPosition, api.Add(gap, 2))
		marginCollateralIndex = api.Add(marginCollateralIndex, 1)
	}

	gap = api.Add(gap, marginTierRatiosLen)
	portfolioMarginCollateralIndex := userAssets.PortfolioMarginCollateralIndex
	for p := 0; p < 2; p++ {
		queries[p*numOfTierRatioFields+0+12] = api.Add(api.Mul(portfolioMarginCollateralIndex, 3), gap)
		queries[p*numOfTierRatioFields+1+12] = api.Add(api.Mul(portfolioMarginCollateralIndex, 3), api.Add(gap, 1))
		queries[p*numOfTierRatioFields+2+12] = api.Add(api.Mul(portfolioMarginCollateralIndex, 3), api.Add(gap, 2))
		portfolioMarginCollateralIndex = api.Add(portfolioMarginCollateralIndex, 1)
	}

	results := tierRatiosTable.Lookup(queries...)

	vipLoanCollateralValue := api.Mul(userVipLoanCollateral, assetPrice)
	// results[0] is less than 2^128 which is constrainted in the GenerateRapidArithmeticForCollateral
	cr := api.CmpNOp(vipLoanCollateralValue, results[0], 128, true)
	// cr only can be 0 or 1
	// cr is 0 in the special case that userAssets.VipLoanCollateral is 0;
	api.AssertIsEqual(cr, api.Select(api.IsZero(vipLoanCollateralValue), 0, 1))
	// results[3] is the upper boundary value
	upperBoundaryValue := api.Select(api.IsZero(userAssets.VipLoanCollateralFlag), results[3], utils.MaxTierBoundaryValueFr)
	api.AssertIsLessOrEqualNOp(vipLoanCollateralValue, upperBoundaryValue, 128, true)
	// results[4] is ratio of upper boundary value
	// diffValue = (vipLoanCollateralValue - lower boundary value) * ratio
	diffValue := api.Mul(api.Sub(vipLoanCollateralValue, results[0]), results[4])
	quotient := CheckAndGetIntegerDivisionRes(api, r, diffValue)
	// Check diffValue is 
	// results[2] is the precomputed value of lower boundary value
	collateralValueRes[0] = api.Select(api.IsZero(userAssets.VipLoanCollateralFlag), api.Add(results[2], quotient), results[5])

	marginCollateralValue := api.Mul(userMarginCollateral, assetPrice)
	cr = api.CmpNOp(marginCollateralValue, results[6], 128, true)
	api.AssertIsEqual(cr, api.Select(api.IsZero(marginCollateralValue), 0, 1))
	upperBoundaryValue = api.Select(api.IsZero(userAssets.MarginCollateralFlag), results[9], utils.MaxTierBoundaryValueFr)
	api.AssertIsLessOrEqualNOp(marginCollateralValue, upperBoundaryValue, 128, true)
	collateralValueRes[1] = marginCollateralValue
	diffValue = api.Mul(api.Sub(marginCollateralValue, results[6]), results[10])
	quotient = CheckAndGetIntegerDivisionRes(api, r, diffValue)
	collateralValueRes[1] = api.Select(api.IsZero(userAssets.MarginCollateralFlag), api.Add(results[8], quotient), results[11])

	portfolioMarginCollateralValue := api.Mul(userPortfolioMarginCollateral, assetPrice)
	cr = api.CmpNOp(portfolioMarginCollateralValue, results[12], 128, true)
	api.AssertIsEqual(cr, api.Select(api.IsZero(portfolioMarginCollateralValue), 0, 1))
	upperBoundaryValue = api.Select(api.IsZero(userAssets.PortfolioMarginCollateralFlag), results[15], utils.MaxTierBoundaryValueFr)
	api.AssertIsLessOrEqualNOp(portfolioMarginCollateralValue, upperBoundaryValue, 128, true)
	collateralValueRes[2] = portfolioMarginCollateralValue
	diffValue = api.Mul(api.Sub(portfolioMarginCollateralValue, results[12]), results[16])
	quotient = CheckAndGetIntegerDivisionRes(api, r, diffValue)
	collateralValueRes[2] = api.Select(api.IsZero(userAssets.PortfolioMarginCollateralFlag), api.Add(results[14], quotient), results[17])

	return collateralValueRes
}

func CheckAndGetIntegerDivisionRes(api API, r frontend.Rangechecker, dividend Variable) (quotient Variable) {
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

func ConstructTierRatiosLookupTable(api API, cexAssetInfo []CexAssetInfo) *logderivlookup.Table {
	t := logderivlookup.New(api)
	for i := 0; i < len(cexAssetInfo); i++ {
		// dummy tier ratio
		for i := 0; i < 3; i++ {
			t.Insert(0)
		}
		for j := 0; j < len(cexAssetInfo[i].VipLoanRatios); j++ {
			t.Insert(cexAssetInfo[i].VipLoanRatios[j].BoundaryValue)
			t.Insert(cexAssetInfo[i].VipLoanRatios[j].Ratio)
			t.Insert(cexAssetInfo[i].VipLoanRatios[j].PrecomputedValue)
		}

		for i := 0; i < 3; i++ {
			t.Insert(0)
		}
		for j := 0; j < len(cexAssetInfo[i].MarginRatios); j++ {
			t.Insert(cexAssetInfo[i].MarginRatios[j].BoundaryValue)
			t.Insert(cexAssetInfo[i].MarginRatios[j].Ratio)
			t.Insert(cexAssetInfo[i].MarginRatios[j].PrecomputedValue)
		}

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

func CalcAndSetCollateralInfo(assetIndex int, ua *UserAssetInfo, um *utils.AccountAsset, cexInfo []utils.CexAssetInfo) {
	p := cexInfo[assetIndex]
	assestPrice := new(big.Int).SetUint64(p.BasePrice)
	userVipLoanCollateral := new(big.Int).SetUint64(um.VipLoan)
	userVipLoanCollateral.Mul(userVipLoanCollateral, assestPrice)
	userMarginCollateral := new(big.Int).SetUint64(um.Margin)
	userMarginCollateral.Mul(userMarginCollateral, assestPrice)
	userPortfolioMarginCollateral := new(big.Int).SetUint64(um.PortfolioMargin)
	userPortfolioMarginCollateral.Mul(userPortfolioMarginCollateral, assestPrice)

	var findFlag bool = false
	for i := 0; i < len(p.VipLoanRatios); i++ {
		if userVipLoanCollateral.Cmp(p.VipLoanRatios[i].BoundaryValue) <= 0 {
			ua.VipLoanCollateralIndex = i
			ua.VipLoanCollateralFlag = 0
			findFlag = true
			break
		}
	}
	if !findFlag {
		ua.VipLoanCollateralIndex = len(p.VipLoanRatios) - 1
		ua.VipLoanCollateralFlag = 1
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