package circuit

import (
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/consensys/gnark/std/hash/poseidon"

	"github.com/consensys/gnark/std/lookup/logderivlookup"
	"github.com/consensys/gnark/std/rangecheck"
)

type BatchCreateUserCircuit struct {
	BatchCommitment           Variable `gnark:",public"`
	BeforeAccountTreeRoot     Variable
	AfterAccountTreeRoot      Variable
	BeforeCEXAssetsCommitment Variable
	AfterCEXAssetsCommitment  Variable
	BeforeCexAssets           []CexAssetInfo
	CreateUserOps             []CreateUserOperation
}

func NewVerifyBatchCreateUserCircuit(commitment []byte) *BatchCreateUserCircuit {
	var v BatchCreateUserCircuit
	v.BatchCommitment = commitment
	return &v
}

func NewBatchCreateUserCircuit(userAssetCounts uint32, allAssetCounts uint32, batchCounts uint32) *BatchCreateUserCircuit {
	var circuit BatchCreateUserCircuit
	circuit.BatchCommitment = 0
	circuit.BeforeAccountTreeRoot = 0
	circuit.AfterAccountTreeRoot = 0
	circuit.BeforeCEXAssetsCommitment = 0
	circuit.AfterCEXAssetsCommitment = 0
	circuit.BeforeCexAssets = make([]CexAssetInfo, allAssetCounts)
	for i := uint32(0); i < allAssetCounts; i++ {
		circuit.BeforeCexAssets[i] = CexAssetInfo{
			TotalEquity:               0,
			TotalDebt:                 0,
			BasePrice:                 0,
			LoanCollateral:            0,
			MarginCollateral:          0,
			PortfolioMarginCollateral: 0,
			LoanRatios:                make([]TierRatio, utils.TierCount),
			MarginRatios:              make([]TierRatio, utils.TierCount),
			PortfolioMarginRatios:     make([]TierRatio, utils.TierCount),
		}
		for j := uint32(0); j < utils.TierCount; j++ {
			circuit.BeforeCexAssets[i].LoanRatios[j] = TierRatio{
				BoundaryValue:    0,
				Ratio:            0,
				PrecomputedValue: 0,
			}
			circuit.BeforeCexAssets[i].MarginRatios[j] = TierRatio{
				BoundaryValue:    0,
				Ratio:            0,
				PrecomputedValue: 0,
			}
			circuit.BeforeCexAssets[i].PortfolioMarginRatios[j] = TierRatio{
				BoundaryValue:    0,
				Ratio:            0,
				PrecomputedValue: 0,
			}
		}
	}
	circuit.CreateUserOps = make([]CreateUserOperation, batchCounts)
	for i := uint32(0); i < batchCounts; i++ {
		circuit.CreateUserOps[i] = CreateUserOperation{
			BeforeAccountTreeRoot: 0,
			AfterAccountTreeRoot:  0,
			Assets:                make([]UserAssetInfo, userAssetCounts),
			AssetsForUpdateCex:    make([]UserAssetMeta, allAssetCounts),
			AccountIndex:          0,
			AccountIdHash:         0,
			AccountProof:          [utils.AccountTreeDepth]Variable{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		}
		for j := uint32(0); j < allAssetCounts; j++ {
			circuit.CreateUserOps[i].AssetsForUpdateCex[j].Debt = 0
			circuit.CreateUserOps[i].AssetsForUpdateCex[j].Equity = 0
			circuit.CreateUserOps[i].AssetsForUpdateCex[j].LoanCollateral = 0
			circuit.CreateUserOps[i].AssetsForUpdateCex[j].MarginCollateral = 0
			circuit.CreateUserOps[i].AssetsForUpdateCex[j].PortfolioMarginCollateral = 0
		}
		for j := uint32(0); j < userAssetCounts; j++ {
			circuit.CreateUserOps[i].Assets[j] = UserAssetInfo{
				AssetIndex:                     j,
				LoanCollateralIndex:            0,
				LoanCollateralFlag:             0,
				MarginCollateralIndex:          0,
				MarginCollateralFlag:           0,
				PortfolioMarginCollateralIndex: 0,
				PortfolioMarginCollateralFlag:  0,
			}
		}
	}
	return &circuit
}

func (b BatchCreateUserCircuit) Define(api API) error {
	// verify whether BatchCommitment is computed correctly
	actualBatchCommitment := poseidon.Poseidon(api, b.BeforeAccountTreeRoot, b.AfterAccountTreeRoot, b.BeforeCEXAssetsCommitment, b.AfterCEXAssetsCommitment)
	api.AssertIsEqual(b.BatchCommitment, actualBatchCommitment)
	countOfCexAsset := getVariableCountOfCexAsset(b.BeforeCexAssets[0])
	cexAssets := make([]Variable, len(b.BeforeCexAssets)*countOfCexAsset)
	afterCexAssets := make([]CexAssetInfo, len(b.BeforeCexAssets))

	r := rangecheck.New(api)
	// verify whether beforeCexAssetsCommitment is computed correctly
	assetPriceTable := logderivlookup.New(api)
	for i := 0; i < len(b.BeforeCexAssets); i++ {
		r.Check(b.BeforeCexAssets[i].TotalEquity, 64)
		r.Check(b.BeforeCexAssets[i].TotalDebt, 64)
		r.Check(b.BeforeCexAssets[i].BasePrice, 64)
		r.Check(b.BeforeCexAssets[i].LoanCollateral, 64)
		r.Check(b.BeforeCexAssets[i].MarginCollateral, 64)
		r.Check(b.BeforeCexAssets[i].PortfolioMarginCollateral, 64)

		fillCexAssetCommitment(api, b.BeforeCexAssets[i], i, cexAssets)
		generateRapidArithmeticForCollateral(api, r, b.BeforeCexAssets[i].LoanRatios)
		generateRapidArithmeticForCollateral(api, r, b.BeforeCexAssets[i].MarginRatios)
		generateRapidArithmeticForCollateral(api, r, b.BeforeCexAssets[i].PortfolioMarginRatios)
		afterCexAssets[i] = b.BeforeCexAssets[i]

		assetPriceTable.Insert(b.BeforeCexAssets[i].BasePrice)
	}
	actualCexAssetsCommitment := poseidon.Poseidon(api, cexAssets...)
	api.AssertIsEqual(b.BeforeCEXAssetsCommitment, actualCexAssetsCommitment)
	api.AssertIsEqual(b.BeforeAccountTreeRoot, b.CreateUserOps[0].BeforeAccountTreeRoot)
	api.AssertIsEqual(b.AfterAccountTreeRoot, b.CreateUserOps[len(b.CreateUserOps)-1].AfterAccountTreeRoot)

	loanTierRatiosTable := constructLoanTierRatiosLookupTable(api, b.BeforeCexAssets)
	marginTierRatiosTable := constructMarginTierRatiosLookupTable(api, b.BeforeCexAssets)
	portfolioMarginTierRatiosTable := constructPortfolioTierRatiosLookupTable(api, b.BeforeCexAssets)
	userAssetIdHashes := make([]Variable, len(b.CreateUserOps)+1)

	userAssetsResults := make([][]Variable, len(b.CreateUserOps))
	userAssetsQueries := make([][]Variable, len(b.CreateUserOps))

	for i := 0; i < len(b.CreateUserOps); i++ {
		accountIndexHelper := accountIdToMerkleHelper(api, b.CreateUserOps[i].AccountIndex)
		verifyMerkleProof(api, b.CreateUserOps[i].BeforeAccountTreeRoot, EmptyAccountLeafNodeHash, b.CreateUserOps[i].AccountProof[:], accountIndexHelper)
		var totalUserEquity Variable = 0
		var totalUserDebt Variable = 0
		userAssets := b.CreateUserOps[i].Assets
		var totalUserCollateralRealValue Variable = 0

		// construct lookup table for user assets
		userAssetsLookupTable := logderivlookup.New(api)
		for j := 0; j < len(b.CreateUserOps[i].AssetsForUpdateCex); j++ {
			userAssetsLookupTable.Insert(b.CreateUserOps[i].AssetsForUpdateCex[j].Equity)
			userAssetsLookupTable.Insert(b.CreateUserOps[i].AssetsForUpdateCex[j].Debt)
			userAssetsLookupTable.Insert(b.CreateUserOps[i].AssetsForUpdateCex[j].LoanCollateral)
			userAssetsLookupTable.Insert(b.CreateUserOps[i].AssetsForUpdateCex[j].MarginCollateral)
			userAssetsLookupTable.Insert(b.CreateUserOps[i].AssetsForUpdateCex[j].PortfolioMarginCollateral)
		}

		// To check all the user assetIndexes are unique to each other.
		// If the user assetIndex is increasing, Then all the assetIndexes are unique
		for j := 0; j < len(userAssets)-1; j++ {
			r.Check(userAssets[j].AssetIndex, 16)
			cr := api.CmpNOp(userAssets[j+1].AssetIndex, userAssets[j].AssetIndex, 16, true)
			api.AssertIsEqual(cr, 1)
		}

		// one Variable can store 15 assetIds, one assetId is less than 16 bits
		assetIdsToVariables := make([]Variable, (len(userAssets)+14)/15)
		for j := 0; j < len(assetIdsToVariables); j++ {
			var v Variable = 0
			for p := j * 15; p < (j+1)*15 && p < len(userAssets); p++ {
				v = api.Add(v, api.Mul(userAssets[p].AssetIndex, utils.PowersOfSixteenBits[p%15]))
			}
			assetIdsToVariables[j] = v
		}
		userAssetIdHashes[i] = poseidon.Poseidon(api, assetIdsToVariables...)

		// construct query to get user assets
		userAssetsQueries[i] = make([]Variable, len(userAssets)*5)
		assetPriceQueries := make([]Variable, len(userAssets))
		numOfAssetsFields := 6
		for j := 0; j < len(userAssets); j++ {
			p := api.Mul(userAssets[j].AssetIndex, 5)
			for k := 0; k < 5; k++ {
				userAssetsQueries[i][j*5+k] = api.Add(p, k)
			}
			assetPriceQueries[j] = userAssets[j].AssetIndex
		}
		userAssetsResults[i] = userAssetsLookupTable.Lookup(userAssetsQueries[i]...)
		assetPriceResponses := assetPriceTable.Lookup(assetPriceQueries...)

		flattenAssetFieldsForHash := make([]Variable, len(userAssets)*numOfAssetsFields)
		for j := 0; j < len(userAssets); j++ {
			// Equity
			userEquity := userAssetsResults[i][j*5]
			r.Check(userEquity, 64)
			// Debt
			userDebt := userAssetsResults[i][j*5+1]
			r.Check(userDebt, 64)
			// LoanCollateral
			userLoanCollateral := userAssetsResults[i][j*5+2]
			r.Check(userLoanCollateral, 64)
			// MarginCollateral
			userMarginCollateral := userAssetsResults[i][j*5+3]
			r.Check(userMarginCollateral, 64)
			// PortfolioMarginCollateral
			userPortfolioMarginCollateral := userAssetsResults[i][j*5+4]
			r.Check(userPortfolioMarginCollateral, 64)

			flattenAssetFieldsForHash[j*numOfAssetsFields] = userAssets[j].AssetIndex
			flattenAssetFieldsForHash[j*numOfAssetsFields+1] = userEquity
			flattenAssetFieldsForHash[j*numOfAssetsFields+2] = userDebt
			flattenAssetFieldsForHash[j*numOfAssetsFields+3] = userLoanCollateral
			flattenAssetFieldsForHash[j*numOfAssetsFields+4] = userMarginCollateral
			flattenAssetFieldsForHash[j*numOfAssetsFields+5] = userPortfolioMarginCollateral

			assetTotalCollateral := api.Add(userLoanCollateral, userMarginCollateral, userPortfolioMarginCollateral)
			r.Check(assetTotalCollateral, 64)
			api.AssertIsLessOrEqualNOp(assetTotalCollateral, userEquity, 64, true)

			loanRealValue := getAndCheckTierRatiosQueryResults(api, r, loanTierRatiosTable, userAssets[j].AssetIndex,
				userLoanCollateral,
				userAssets[j].LoanCollateralIndex,
				userAssets[j].LoanCollateralFlag,
				assetPriceResponses[j],
				3*(len(b.BeforeCexAssets[j].LoanRatios)+1))

			marginRealValue := getAndCheckTierRatiosQueryResults(api, r, marginTierRatiosTable, userAssets[j].AssetIndex,
				userMarginCollateral,
				userAssets[j].MarginCollateralIndex,
				userAssets[j].MarginCollateralFlag,
				assetPriceResponses[j],
				3*(len(b.BeforeCexAssets[j].MarginRatios)+1))

			portfolioMarginRealValue := getAndCheckTierRatiosQueryResults(api, r, portfolioMarginTierRatiosTable, userAssets[j].AssetIndex,
				userPortfolioMarginCollateral,
				userAssets[j].PortfolioMarginCollateralIndex,
				userAssets[j].PortfolioMarginCollateralFlag,
				assetPriceResponses[j],
				3*(len(b.BeforeCexAssets[j].PortfolioMarginRatios)+1))

			totalUserCollateralRealValue = api.Add(totalUserCollateralRealValue, loanRealValue, marginRealValue, portfolioMarginRealValue)

			totalUserEquity = api.Add(totalUserEquity, api.Mul(userEquity, assetPriceResponses[j]))
			totalUserDebt = api.Add(totalUserDebt, api.Mul(userDebt, assetPriceResponses[j]))
		}

		for j := 0; j < len(b.CreateUserOps[i].AssetsForUpdateCex); j++ {
			afterCexAssets[j].TotalEquity = api.Add(afterCexAssets[j].TotalEquity, b.CreateUserOps[i].AssetsForUpdateCex[j].Equity)
			afterCexAssets[j].TotalDebt = api.Add(afterCexAssets[j].TotalDebt, b.CreateUserOps[i].AssetsForUpdateCex[j].Debt)
			afterCexAssets[j].LoanCollateral = api.Add(afterCexAssets[j].LoanCollateral, b.CreateUserOps[i].AssetsForUpdateCex[j].LoanCollateral)
			afterCexAssets[j].MarginCollateral = api.Add(afterCexAssets[j].MarginCollateral, b.CreateUserOps[i].AssetsForUpdateCex[j].MarginCollateral)
			afterCexAssets[j].PortfolioMarginCollateral = api.Add(afterCexAssets[j].PortfolioMarginCollateral, b.CreateUserOps[i].AssetsForUpdateCex[j].PortfolioMarginCollateral)
		}

		// make sure user's total Debt is less or equal than total collateral
		r.Check(totalUserDebt, 128)
		r.Check(totalUserCollateralRealValue, 128)
		api.AssertIsLessOrEqualNOp(totalUserDebt, totalUserCollateralRealValue, 128, true)
		userAssetsCommitment := computeUserAssetsCommitment(api, flattenAssetFieldsForHash)
		accountHash := poseidon.Poseidon(api, b.CreateUserOps[i].AccountIdHash, totalUserEquity, totalUserDebt, totalUserCollateralRealValue, userAssetsCommitment)
		actualAccountTreeRoot := updateMerkleProof(api, accountHash, b.CreateUserOps[i].AccountProof[:], accountIndexHelper)
		api.AssertIsEqual(actualAccountTreeRoot, b.CreateUserOps[i].AfterAccountTreeRoot)
	}

	// make sure user assets contains all non-zero assets of AssetsForUpdateCex
	// use random linear combination to check, the random number is poseidon hash of two elements:
	// 1. the public input of circuit -- batch commitment
	// 2. the poseidon hash of user assets index

	userAssetIdHashes[len(b.CreateUserOps)] = b.BatchCommitment
	randomChallenge := poseidon.Poseidon(api, userAssetIdHashes...)
	powersOfRandomChallenge := make([]Variable, 5*len(b.BeforeCexAssets))
	powersOfRandomChallenge[0] = randomChallenge
	powersOfRandomChallengeLookupTable := logderivlookup.New(api)
	powersOfRandomChallengeLookupTable.Insert(randomChallenge)
	for i := 1; i < len(powersOfRandomChallenge); i++ {
		powersOfRandomChallenge[i] = api.Mul(powersOfRandomChallenge[i-1], randomChallenge)
		powersOfRandomChallengeLookupTable.Insert(powersOfRandomChallenge[i])
	}

	for i := 0; i < len(b.CreateUserOps); i++ {
		powersOfRCResults := powersOfRandomChallengeLookupTable.Lookup(userAssetsQueries[i]...)
		var sumA Variable = 0
		for j := 0; j < len(powersOfRCResults); j++ {
			sumA = api.Add(sumA, api.Mul(powersOfRCResults[j], userAssetsResults[i][j]))
		}

		var sumB Variable = 0
		for j := 0; j < len(b.CreateUserOps[i].AssetsForUpdateCex); j++ {
			sumB = api.Add(sumB, api.Mul(b.CreateUserOps[i].AssetsForUpdateCex[j].Equity, powersOfRandomChallenge[5*j]))
			sumB = api.Add(sumB, api.Mul(b.CreateUserOps[i].AssetsForUpdateCex[j].Debt, powersOfRandomChallenge[5*j+1]))
			sumB = api.Add(sumB, api.Mul(b.CreateUserOps[i].AssetsForUpdateCex[j].LoanCollateral, powersOfRandomChallenge[5*j+2]))
			sumB = api.Add(sumB, api.Mul(b.CreateUserOps[i].AssetsForUpdateCex[j].MarginCollateral, powersOfRandomChallenge[5*j+3]))
			sumB = api.Add(sumB, api.Mul(b.CreateUserOps[i].AssetsForUpdateCex[j].PortfolioMarginCollateral, powersOfRandomChallenge[5*j+4]))
		}
		api.AssertIsEqual(sumA, sumB)
	}
	tempAfterCexAssets := make([]Variable, len(b.BeforeCexAssets)*countOfCexAsset)
	for j := 0; j < len(b.BeforeCexAssets); j++ {
		r.Check(afterCexAssets[j].TotalEquity, 64)
		r.Check(afterCexAssets[j].TotalDebt, 64)
		r.Check(afterCexAssets[j].LoanCollateral, 64)
		r.Check(afterCexAssets[j].MarginCollateral, 64)
		r.Check(afterCexAssets[j].PortfolioMarginCollateral, 64)

		fillCexAssetCommitment(api, afterCexAssets[j], j, tempAfterCexAssets)
	}

	// verify AfterCEXAssetsCommitment is computed correctly
	actualAfterCEXAssetsCommitment := poseidon.Poseidon(api, tempAfterCexAssets...)
	api.AssertIsEqual(actualAfterCEXAssetsCommitment, b.AfterCEXAssetsCommitment)
	api.Println("actualAfterCEXAssetsCommitment: ", actualAfterCEXAssetsCommitment)
	api.Println("AfterCEXAssetsCommitment: ", b.AfterCEXAssetsCommitment)
	for i := 0; i < len(b.CreateUserOps)-1; i++ {
		api.AssertIsEqual(b.CreateUserOps[i].AfterAccountTreeRoot, b.CreateUserOps[i+1].BeforeAccountTreeRoot)
	}
	return nil
}

func copyTierRatios(dst []TierRatio, src []utils.TierRatio) {
	for i := 0; i < len(dst); i++ {
		dst[i].BoundaryValue = src[i].BoundaryValue
		dst[i].Ratio = src[i].Ratio
		dst[i].PrecomputedValue = src[i].PrecomputedValue
	}

}

func SetBatchCreateUserCircuitWitness(batchWitness *utils.BatchCreateUserWitness) (witness *BatchCreateUserCircuit, err error) {
	witness = &BatchCreateUserCircuit{
		BatchCommitment:           batchWitness.BatchCommitment,
		BeforeAccountTreeRoot:     batchWitness.BeforeAccountTreeRoot,
		AfterAccountTreeRoot:      batchWitness.AfterAccountTreeRoot,
		BeforeCEXAssetsCommitment: batchWitness.BeforeCEXAssetsCommitment,
		AfterCEXAssetsCommitment:  batchWitness.AfterCEXAssetsCommitment,
		BeforeCexAssets:           make([]CexAssetInfo, len(batchWitness.BeforeCexAssets)),
		CreateUserOps:             make([]CreateUserOperation, len(batchWitness.CreateUserOps)),
	}

	for i := 0; i < len(witness.BeforeCexAssets); i++ {
		witness.BeforeCexAssets[i].TotalEquity = batchWitness.BeforeCexAssets[i].TotalEquity
		witness.BeforeCexAssets[i].TotalDebt = batchWitness.BeforeCexAssets[i].TotalDebt
		witness.BeforeCexAssets[i].BasePrice = batchWitness.BeforeCexAssets[i].BasePrice
		witness.BeforeCexAssets[i].LoanCollateral = batchWitness.BeforeCexAssets[i].LoanCollateral
		witness.BeforeCexAssets[i].MarginCollateral = batchWitness.BeforeCexAssets[i].MarginCollateral
		witness.BeforeCexAssets[i].PortfolioMarginCollateral = batchWitness.BeforeCexAssets[i].PortfolioMarginCollateral
		witness.BeforeCexAssets[i].LoanRatios = make([]TierRatio, len(batchWitness.BeforeCexAssets[i].LoanRatios))
		copyTierRatios(witness.BeforeCexAssets[i].LoanRatios, batchWitness.BeforeCexAssets[i].LoanRatios[:])
		witness.BeforeCexAssets[i].MarginRatios = make([]TierRatio, len(batchWitness.BeforeCexAssets[i].MarginRatios))
		copyTierRatios(witness.BeforeCexAssets[i].MarginRatios, batchWitness.BeforeCexAssets[i].MarginRatios[:])
		witness.BeforeCexAssets[i].PortfolioMarginRatios = make([]TierRatio, len(batchWitness.BeforeCexAssets[i].PortfolioMarginRatios))
		copyTierRatios(witness.BeforeCexAssets[i].PortfolioMarginRatios, batchWitness.BeforeCexAssets[i].PortfolioMarginRatios[:])
	}

	cexAssetsCount := len(witness.BeforeCexAssets)
	// Decide the assets count for user according to the first user,
	// because the assets count for all users in a batch are the same
	// and the rest of the users in the batch may be padding accounts
	targetCounts := utils.GetNonEmptyAssetsCountOfUser(batchWitness.CreateUserOps[0].Assets)
	for i := 0; i < len(witness.CreateUserOps); i++ {
		witness.CreateUserOps[i].BeforeAccountTreeRoot = batchWitness.CreateUserOps[i].BeforeAccountTreeRoot
		witness.CreateUserOps[i].AfterAccountTreeRoot = batchWitness.CreateUserOps[i].AfterAccountTreeRoot
		witness.CreateUserOps[i].AssetsForUpdateCex = make([]UserAssetMeta, cexAssetsCount)

		existingKeys := make([]int, 0)
		for j := 0; j < len(batchWitness.CreateUserOps[i].Assets); j++ {
			u := batchWitness.CreateUserOps[i].Assets[j]
			userAsset := UserAssetMeta{
				Equity:                    u.Equity,
				Debt:                      u.Debt,
				LoanCollateral:            u.Loan,
				MarginCollateral:          u.Margin,
				PortfolioMarginCollateral: u.PortfolioMargin,
			}

			witness.CreateUserOps[i].AssetsForUpdateCex[j] = userAsset

			if !utils.IsAssetEmpty(&u) {
				existingKeys = append(existingKeys, int(u.Index))
			}
		}
		paddingCounts := targetCounts - len(existingKeys)
		witness.CreateUserOps[i].Assets = make([]UserAssetInfo, targetCounts)
		currentPaddingCounts := 0
		currentAssetIndex := 0
		index := 0
		for _, v := range existingKeys {
			if currentPaddingCounts < paddingCounts {
				for k := currentAssetIndex; k < v; k++ {
					currentPaddingCounts += 1
					witness.CreateUserOps[i].Assets[index] = UserAssetInfo{
						AssetIndex:                     uint32(k),
						LoanCollateralIndex:            0,
						LoanCollateralFlag:             0,
						MarginCollateralIndex:          0,
						MarginCollateralFlag:           0,
						PortfolioMarginCollateralIndex: 0,
						PortfolioMarginCollateralFlag:  0,
					}
					index += 1
					if currentPaddingCounts >= paddingCounts {
						break
					}
				}
			}
			var uAssetInfo UserAssetInfo
			uAssetInfo.AssetIndex = uint32(v)
			calcAndSetCollateralInfo(v, &uAssetInfo, &batchWitness.CreateUserOps[i].Assets[v], batchWitness.BeforeCexAssets)
			witness.CreateUserOps[i].Assets[index] = uAssetInfo
			index += 1
			currentAssetIndex = v + 1
		}
		for k := index; k < targetCounts; k++ {
			witness.CreateUserOps[i].Assets[k] = UserAssetInfo{
				AssetIndex:                     uint32(currentAssetIndex),
				LoanCollateralIndex:            0,
				LoanCollateralFlag:             0,
				MarginCollateralIndex:          0,
				MarginCollateralFlag:           0,
				PortfolioMarginCollateralIndex: 0,
				PortfolioMarginCollateralFlag:  0,
			}
			currentAssetIndex += 1
		}
		witness.CreateUserOps[i].AccountIdHash = batchWitness.CreateUserOps[i].AccountIdHash
		witness.CreateUserOps[i].AccountIndex = batchWitness.CreateUserOps[i].AccountIndex
		for j := 0; j < len(witness.CreateUserOps[i].AccountProof); j++ {
			witness.CreateUserOps[i].AccountProof[j] = batchWitness.CreateUserOps[i].AccountProof[j]
		}
	}
	return witness, nil
}
