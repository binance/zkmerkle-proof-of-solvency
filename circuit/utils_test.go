package circuit

import (
	"fmt"
	"testing"
	"time"

	"math/big"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/constraint/solver"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/std/lookup/logderivlookup"
	"github.com/consensys/gnark/std/rangecheck"
)

type MockCollateralCircuit struct {
	UAssetInfo     []UserAssetInfo
	UAssetMataInfo []UserAssetMeta
	AssetId        []int

	CAssetInfo []CexAssetInfo

	ExpectedLoanCollateral            []Variable
	ExpectedMarginCollateral          []Variable
	ExpectedPortfolioMarginCollateral []Variable
}

func (circuit MockCollateralCircuit) Define(api API) error {
	r := rangecheck.New(api)
	for i := 0; i < len(circuit.CAssetInfo); i++ {
		generateRapidArithmeticForCollateral(api, r, circuit.CAssetInfo[i].LoanRatios)
		generateRapidArithmeticForCollateral(api, r, circuit.CAssetInfo[i].MarginRatios)
		generateRapidArithmeticForCollateral(api, r, circuit.CAssetInfo[i].PortfolioMarginRatios)
	}
	t0 := constructLoanTierRatiosLookupTable(api, circuit.CAssetInfo)
	t1 := constructMarginTierRatiosLookupTable(api, circuit.CAssetInfo)
	t2 := constructPortfolioTierRatiosLookupTable(api, circuit.CAssetInfo)

	for i := 0; i < len(circuit.UAssetInfo); i++ {
		realLoanCollateralValue := getAndCheckTierRatiosQueryResults(api, r, t0, circuit.UAssetInfo[i].AssetIndex,
			circuit.UAssetMataInfo[i].LoanCollateral,
			circuit.UAssetInfo[i].LoanCollateralIndex,
			circuit.UAssetInfo[i].LoanCollateralFlag,
			circuit.CAssetInfo[circuit.AssetId[i]].BasePrice,
			3*(len(circuit.CAssetInfo[circuit.AssetId[i]].LoanRatios)+1))

		realMarginCollateralValue := getAndCheckTierRatiosQueryResults(api, r, t1, circuit.UAssetInfo[i].AssetIndex,
			circuit.UAssetMataInfo[i].MarginCollateral,
			circuit.UAssetInfo[i].MarginCollateralIndex,
			circuit.UAssetInfo[i].MarginCollateralFlag,
			circuit.CAssetInfo[circuit.AssetId[i]].BasePrice,
			3*(len(circuit.CAssetInfo[circuit.AssetId[i]].MarginRatios)+1))

		realPortfolioMarginCollateralValue := getAndCheckTierRatiosQueryResults(api, r, t2, circuit.UAssetInfo[i].AssetIndex,
			circuit.UAssetMataInfo[i].PortfolioMarginCollateral,
			circuit.UAssetInfo[i].PortfolioMarginCollateralIndex,
			circuit.UAssetInfo[i].PortfolioMarginCollateralFlag,
			circuit.CAssetInfo[circuit.AssetId[i]].BasePrice,
			3*(len(circuit.CAssetInfo[circuit.AssetId[i]].PortfolioMarginRatios)+1))

		api.AssertIsEqual(circuit.ExpectedLoanCollateral[i], realLoanCollateralValue)
		api.AssertIsEqual(circuit.ExpectedMarginCollateral[i], realMarginCollateralValue)
		api.AssertIsEqual(circuit.ExpectedPortfolioMarginCollateral[i], realPortfolioMarginCollateralValue)
	}
	return nil
}

func TestMockCollateralCircuit(t *testing.T) {
	var circuit MockCollateralCircuit
	circuit.CAssetInfo = make([]CexAssetInfo, 5)
	for i := 0; i < len(circuit.CAssetInfo); i++ {
		circuit.CAssetInfo[i].LoanRatios = make([]TierRatio, 10)
		circuit.CAssetInfo[i].MarginRatios = make([]TierRatio, 10)
		circuit.CAssetInfo[i].PortfolioMarginRatios = make([]TierRatio, 10)
	}
	circuit.AssetId = make([]int, 2)
	circuit.UAssetInfo = make([]UserAssetInfo, 2)
	circuit.UAssetMataInfo = make([]UserAssetMeta, 2)
	circuit.ExpectedLoanCollateral = make([]Variable, 2)
	circuit.ExpectedMarginCollateral = make([]Variable, 2)
	circuit.ExpectedPortfolioMarginCollateral = make([]Variable, 2)

	solver.RegisterHint(IntegerDivision)

	oR1cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit, frontend.IgnoreUnconstrainedInputs())
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("constraints number is ", oR1cs.GetNbConstraints())

	var circuit2 MockCollateralCircuit
	circuit2.CAssetInfo = make([]CexAssetInfo, 5)
	for i := 0; i < len(circuit2.CAssetInfo); i++ {
		circuit2.CAssetInfo[i].TotalEquity = 0
		circuit2.CAssetInfo[i].TotalDebt = 0
		circuit2.CAssetInfo[i].BasePrice = 1
		circuit2.CAssetInfo[i].LoanCollateral = 0
		circuit2.CAssetInfo[i].MarginCollateral = 0
		circuit2.CAssetInfo[i].PortfolioMarginCollateral = 0

		circuit2.CAssetInfo[i].LoanRatios = make([]TierRatio, 10)
		for j := 0; j < 10; j++ {
			circuit2.CAssetInfo[i].LoanRatios[j].BoundaryValue = 10000 * (j + 1)
			ratio := 100 - j*10
			circuit2.CAssetInfo[i].LoanRatios[j].Ratio = ratio
			circuit2.CAssetInfo[i].LoanRatios[j].PrecomputedValue = 0
		}

		circuit2.CAssetInfo[i].MarginRatios = make([]TierRatio, 10)
		for j := 0; j < 10; j++ {
			circuit2.CAssetInfo[i].MarginRatios[j].BoundaryValue = 20001 * (j + 1)
			ratio := 100 - j*9
			circuit2.CAssetInfo[i].MarginRatios[j].Ratio = ratio
			circuit2.CAssetInfo[i].MarginRatios[j].PrecomputedValue = 0
		}

		circuit2.CAssetInfo[i].PortfolioMarginRatios = make([]TierRatio, 10)
		for j := 0; j < 10; j++ {
			circuit2.CAssetInfo[i].PortfolioMarginRatios[j].BoundaryValue = 30000 * (j + 1)
			ratio := 100 - j*8
			circuit2.CAssetInfo[i].PortfolioMarginRatios[j].Ratio = ratio
			circuit2.CAssetInfo[i].PortfolioMarginRatios[j].PrecomputedValue = 0
		}
	}

	circuit2.AssetId = []int{0, 1}
	circuit2.UAssetInfo = make([]UserAssetInfo, 2)
	circuit2.UAssetMataInfo = make([]UserAssetMeta, 2)
	circuit2.ExpectedMarginCollateral = make([]Variable, 2)
	circuit2.ExpectedLoanCollateral = make([]Variable, 2)
	circuit2.ExpectedPortfolioMarginCollateral = make([]Variable, 2)

	circuit2.UAssetMataInfo[0].Equity = 0
	circuit2.UAssetMataInfo[0].Debt = 0
	circuit2.UAssetMataInfo[0].LoanCollateral = 9000
	circuit2.UAssetInfo[0].AssetIndex = 0
	circuit2.UAssetInfo[0].LoanCollateralIndex = 0
	circuit2.UAssetInfo[0].LoanCollateralFlag = 0
	circuit2.ExpectedLoanCollateral[0] = 9000

	circuit2.UAssetMataInfo[0].MarginCollateral = 39000
	circuit2.UAssetInfo[0].MarginCollateralIndex = 1
	circuit2.UAssetInfo[0].MarginCollateralFlag = 0
	circuit2.ExpectedMarginCollateral[0] = 37290

	circuit2.UAssetMataInfo[0].PortfolioMarginCollateral = 300100
	circuit2.UAssetInfo[0].PortfolioMarginCollateralIndex = 9
	circuit2.UAssetInfo[0].PortfolioMarginCollateralFlag = 1
	circuit2.ExpectedPortfolioMarginCollateral[0] = 192000

	circuit2.UAssetMataInfo[1].Equity = 0
	circuit2.UAssetMataInfo[1].Debt = 0
	circuit2.UAssetMataInfo[1].LoanCollateral = 100001
	circuit2.UAssetInfo[1].AssetIndex = 1
	circuit2.UAssetInfo[1].LoanCollateralIndex = 9
	circuit2.UAssetInfo[1].LoanCollateralFlag = 1
	circuit2.ExpectedLoanCollateral[1] = 55000

	circuit2.UAssetMataInfo[1].MarginCollateral = 10000
	circuit2.UAssetInfo[1].MarginCollateralIndex = 0
	circuit2.UAssetInfo[1].MarginCollateralFlag = 0
	circuit2.ExpectedMarginCollateral[1] = 10000

	circuit2.UAssetMataInfo[1].PortfolioMarginCollateral = 200000
	circuit2.UAssetInfo[1].PortfolioMarginCollateralIndex = 6
	circuit2.UAssetInfo[1].PortfolioMarginCollateralFlag = 0
	circuit2.ExpectedPortfolioMarginCollateral[1] = 154400

	witness, err := frontend.NewWitness(&circuit2, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatal(err)
	}
	// err = oR1cs.IsSolved(witness)
	// if err != nil {
	// 	t.Fatal(err)
	// }

	pk, vk, err := groth16.Setup(oR1cs)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("setup done")
	}
	publicWitness, err := witness.Public()
	if err != nil {
		panic(err)
	} else {
		fmt.Println("public witness")
	}
	startTime := time.Now()
	proof, err := groth16.Prove(oR1cs, pk, witness)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("proof")
	}
	endTime := time.Now()
	fmt.Println("prove time is ", endTime.Sub(startTime))
	err = groth16.Verify(proof, vk, publicWitness)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("verify")
	}
}

type MockCexAssetInfo struct {
	LoanRatios            []Variable
	MarginRatios          []Variable
	PortfolioMarginRatios []Variable
}

type MockUserAssetInfo struct {
	LoanCollateral                 Variable
	LoanCollateralIndex            Variable
	MarginCollateral               Variable
	MarginCollateralIndex          Variable
	PortfolioMarginCollateral      Variable
	PortfolioMarginCollateralIndex Variable
}

type MockUserCircuit struct {
	Assets    []MockUserAssetInfo
	CexAssets []MockCexAssetInfo
}

func (circuit MockUserCircuit) Define(api API) error {

	t0 := logderivlookup.New(api)
	t1 := logderivlookup.New(api)
	t2 := logderivlookup.New(api)
	for i := range circuit.CexAssets {
		for j := range circuit.CexAssets[i].LoanRatios {
			t0.Insert(circuit.CexAssets[i].LoanRatios[j])
		}
		for j := range circuit.CexAssets[i].MarginRatios {
			t1.Insert(circuit.CexAssets[i].MarginRatios[j])
		}
		for j := range circuit.CexAssets[i].PortfolioMarginRatios {
			t2.Insert(circuit.CexAssets[i].PortfolioMarginRatios[j])
		}
	}

	q0 := make([]Variable, 3*len(circuit.Assets))
	q1 := make([]Variable, 3*len(circuit.Assets))
	q2 := make([]Variable, 3*len(circuit.Assets))

	for i := range circuit.Assets {
		q0[3*i] = api.Add(api.Mul(circuit.Assets[i].LoanCollateralIndex, 3), 30*i)
		q0[3*i+1] = api.Add(api.Mul(circuit.Assets[i].LoanCollateralIndex, 3), 30*i+1)
		q0[3*i+2] = api.Add(api.Mul(circuit.Assets[i].LoanCollateralIndex, 3), 30*i+2)

		q1[3*i] = api.Add(api.Mul(circuit.Assets[i].MarginCollateralIndex, 3), 30*i)
		q1[3*i+1] = api.Add(api.Mul(circuit.Assets[i].MarginCollateralIndex, 3), 30*i+1)
		q1[3*i+2] = api.Add(api.Mul(circuit.Assets[i].MarginCollateralIndex, 3), 30*i+2)

		q2[3*i] = api.Add(api.Mul(circuit.Assets[i].PortfolioMarginCollateralIndex, 3), 30*i)
		q2[3*i+1] = api.Add(api.Mul(circuit.Assets[i].PortfolioMarginCollateralIndex, 3), 30*i+1)
		q2[3*i+2] = api.Add(api.Mul(circuit.Assets[i].PortfolioMarginCollateralIndex, 3), 30*i+2)
	}

	r0 := t0.Lookup(q0[:]...)
	r1 := t1.Lookup(q1[:]...)
	r2 := t2.Lookup(q2[:]...)

	for i := range circuit.Assets {
		// BoundaryValue
		api.AssertIsLessOrEqualNOp(circuit.Assets[i].LoanCollateral, r0[3*i], 128, true)
		api.AssertIsLessOrEqualNOp(circuit.Assets[i].LoanCollateral, r0[3*i], 128, true)
		api.AssertIsLessOrEqualNOp(circuit.Assets[i].MarginCollateral, r1[3*i], 128, true)
		api.AssertIsLessOrEqualNOp(circuit.Assets[i].MarginCollateral, r1[3*i], 128, true)
		api.AssertIsLessOrEqualNOp(circuit.Assets[i].PortfolioMarginCollateral, r2[3*i], 128, true)
		api.AssertIsLessOrEqualNOp(circuit.Assets[i].PortfolioMarginCollateral, r2[3*i], 128, true)
	}

	return nil
}

func TestMockUserCircuit(t *testing.T) {

	var circuit MockUserCircuit
	circuit.Assets = make([]MockUserAssetInfo, 350)
	circuit.CexAssets = make([]MockCexAssetInfo, 350)
	for i := 0; i < 350; i++ {
		circuit.CexAssets[i].LoanRatios = make([]Variable, 30)
		circuit.CexAssets[i].MarginRatios = make([]Variable, 30)
		circuit.CexAssets[i].PortfolioMarginRatios = make([]Variable, 30)
	}

	oR1cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit, frontend.IgnoreUnconstrainedInputs())
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("constraints number is ", oR1cs.GetNbConstraints())

	var circuit2 MockUserCircuit
	circuit2.CexAssets = make([]MockCexAssetInfo, 350)
	for i := 0; i < 350; i++ {
		circuit2.CexAssets[i].LoanRatios = make([]Variable, 30)
		for j := 0; j < 10; j++ {
			circuit2.CexAssets[i].LoanRatios[3*j] = 100 * (j + 1)
			circuit2.CexAssets[i].LoanRatios[3*j+1] = 100 * (j + 1)
			circuit2.CexAssets[i].LoanRatios[3*j+2] = 100 * (j + 1)
		}
		circuit2.CexAssets[i].MarginRatios = make([]Variable, 30)
		for j := 0; j < 10; j++ {
			circuit2.CexAssets[i].MarginRatios[3*j] = 100 * (j + 1)
			circuit2.CexAssets[i].MarginRatios[3*j+1] = 100 * (j + 1)
			circuit2.CexAssets[i].MarginRatios[3*j+2] = 100 * (j + 1)
		}
		circuit2.CexAssets[i].PortfolioMarginRatios = make([]Variable, 30)
		for j := 0; j < 10; j++ {
			circuit2.CexAssets[i].PortfolioMarginRatios[3*j] = 100 * (j + 1)
			circuit2.CexAssets[i].PortfolioMarginRatios[3*j+1] = 100 * (j + 1)
			circuit2.CexAssets[i].PortfolioMarginRatios[3*j+2] = 100 * (j + 1)
		}
	}
	circuit2.Assets = make([]MockUserAssetInfo, 350)
	for i := 0; i < 350; i++ {
		circuit2.Assets[i].LoanCollateralIndex = 1
		circuit2.Assets[i].MarginCollateralIndex = 1
		circuit2.Assets[i].PortfolioMarginCollateralIndex = 1
		circuit2.Assets[i].LoanCollateral = 199
		circuit2.Assets[i].MarginCollateral = 200
		circuit2.Assets[i].PortfolioMarginCollateral = 199
	}

	witness, _ := frontend.NewWitness(&circuit2, ecc.BN254.ScalarField())
	err = oR1cs.IsSolved(witness)
	if err != nil {
		t.Fatal(err)
	}

	pk, vk, err := groth16.Setup(oR1cs)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("setup done")
	}
	publicWitness, err := witness.Public()
	if err != nil {
		panic(err)
	} else {
		fmt.Println("public witness")
	}
	startTime := time.Now()
	proof, err := groth16.Prove(oR1cs, pk, witness)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("proof")
	}
	endTime := time.Now()
	fmt.Println("prove time is ", endTime.Sub(startTime))
	err = groth16.Verify(proof, vk, publicWitness)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("verify")
	}
}

// The user has 50 types of assets. Additionally, an array of 350 types of assets, denoted as A, is provided, primarily to update the Cex asset information. At this point, it is necessary to prove that the 50 types of assets include information on all non-zero assets among the 350 types, although they may also include some zero asset information.
// How to ensure the above constraints are correct:
// 1. The 50 types of assets are represented by AssetIndex. Therefore, first convert the information of the 350 types of assets into a lookup table.
// 2. Then, for each asset, look up the corresponding information in the lookup table based on its AssetIndex. The array for queries is denoted as Q, and the query results are denoted as R.
// 3. The following constraint needs to be imposed: the AssetIndex of the 50 types of assets must be unique. This can be ensured by making it an increasing array.
// 4. After this, it can only be proven that the 50 asset indexes exist among the 350 types of assets, but it cannot be proven that all non-zero assets in the 350 types are included in these 50 assets.
// 5. This can be ensured by random linear combination. First, generate a commitment based on the 350 types of assets and the 50 AssetIndexes, denoted as H. Generate H^1, H^2, ..., H^350.
// 6. Then H^Q_0 * R_0 + H^Q_1 * R_1 + ... + H^Q_49 *R _49 = H^1*A_0 + H^2*A_1 + ... + H^350*A_349, If equal, it can be concluded that the 50 types of assets include information on all non-zero assets among the 350 types.

type MockRandomLinearCombinationCircuit struct {
	A         []Variable // AssetIndex
	ExpectedA []Variable
	B         []Variable // UserAssets

	// C []Variable
	// D []Variable  //

	H Variable
}

func CalculatePower(mod *big.Int, in []*big.Int, out []*big.Int) error {
	// in[0] is the base element
	// int[1]... is the power
	for i, v := range in[1:] {
		out[i].Exp(in[0], v, mod)
	}
	return nil
}

func (circuit MockRandomLinearCombinationCircuit) Define(api API) error {
	t0 := logderivlookup.New(api)

	for i := 0; i < len(circuit.B); i++ {
		t0.Insert(circuit.B[i])
	}
	HTable := logderivlookup.New(api)
	HPowers := make([]Variable, 350*5)
	HPowers[0] = circuit.H
	for i := 1; i < len(HPowers); i++ {
		HPowers[i] = api.Mul(HPowers[i-1], circuit.H)
	}
	for i := 0; i < len(HPowers); i++ {
		HTable.Insert(HPowers[i])
	}

	rc := rangecheck.New(api)
	q0 := make([]Variable, 5*len(circuit.A))
	rc.Check(circuit.A[0], 9)
	for i := 1; i < len(circuit.A); i++ {
		rc.Check(circuit.A[0], 9)
		api.AssertIsEqual(api.CmpNOp(circuit.A[i], circuit.A[i-1], 9, true), 1)
	}
	for i := 0; i < len(circuit.A); i++ {
		start := api.Mul(circuit.A[i], 5)
		for j := 0; j < 5; j++ {
			q0[5*i+j] = api.Add(start, j)
		}
	}

	r0 := t0.Lookup(q0[:]...)

	subHTable := HTable.Lookup(q0[:]...)

	var accumulateSumOfR0 Variable = 0
	for i := 0; i < len(r0); i++ {
		accumulateSumOfR0 = api.Add(accumulateSumOfR0, api.Mul(subHTable[i], r0[i]))
	}

	var expectedAccumulateSumOfUserAssets Variable = 0
	for i := 0; i < len(circuit.B); i++ {
		expectedAccumulateSumOfUserAssets = api.Add(expectedAccumulateSumOfUserAssets, api.Mul(circuit.B[i], HPowers[i]))
	}

	api.AssertIsEqual(accumulateSumOfR0, expectedAccumulateSumOfUserAssets)

	for i := 0; i < len(circuit.ExpectedA); i++ {
		api.AssertIsEqual(r0[i], circuit.ExpectedA[i])
	}

	return nil
}

func TestMockRandomLinearCombinationCircuit(t *testing.T) {
	var circuit MockRandomLinearCombinationCircuit
	circuit.A = make([]frontend.Variable, 50)
	circuit.ExpectedA = make([]frontend.Variable, 50*5)
	circuit.B = make([]frontend.Variable, 350*5)

	oR1cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit, frontend.IgnoreUnconstrainedInputs())
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("constraints number is ", oR1cs.GetNbConstraints())

	var circuit2 MockRandomLinearCombinationCircuit
	circuit2.A = make([]frontend.Variable, 50)
	for i := 0; i < len(circuit2.A); i++ {
		circuit2.A[i] = 7 * i
	}
	circuit2.ExpectedA = make([]frontend.Variable, 50*5)
	circuit2.B = make([]frontend.Variable, 350*5)
	c := 0
	for i := 0; i < 350; i++ {
		if i%7 == 0 {
			rr := i / 7
			circuit2.B[5*i] = rr % 2
			circuit2.B[5*i+1] = rr % 3
			circuit2.B[5*i+2] = rr % 4
			circuit2.B[5*i+3] = rr % 5
			circuit2.B[5*i+4] = rr % 6
			circuit2.ExpectedA[5*c] = rr % 2
			circuit2.ExpectedA[5*c+1] = rr % 3
			circuit2.ExpectedA[5*c+2] = rr % 4
			circuit2.ExpectedA[5*c+3] = rr % 5
			circuit2.ExpectedA[5*c+4] = rr % 6
			c += 1
		} else {
			circuit2.B[5*i] = 0
			circuit2.B[5*i+1] = 0
			circuit2.B[5*i+2] = 0
			circuit2.B[5*i+3] = 0
			circuit2.B[5*i+4] = 0
		}
	}
	circuit2.H = 123123123
	witness, _ := frontend.NewWitness(&circuit2, ecc.BN254.ScalarField())
	err = oR1cs.IsSolved(witness)
	if err != nil {
		t.Fatal(err)
	}
}
