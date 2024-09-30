package circuit

import (
	"fmt"
	"testing"

	// "github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"math/big"
	"math/rand"
	"time"

	"encoding/hex"
	"os"

	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bls24-315/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/backend/plonk"
	"github.com/consensys/gnark/backend/witness"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/constraint/solver"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/frontend/cs/scs"
	poseidon2 "github.com/consensys/gnark/std/hash/poseidon"
	"github.com/consensys/gnark/test/unsafekzg"
	"github.com/klauspost/compress/s2"
)

func ConstructR1csAndWitness(provingSystem string) (constraint.ConstraintSystem, witness.Witness, error) {
	solver.RegisterHint(IntegerDivision)
	targetAssetCounts := 30
	totalAssetsCount := 500
	userOpsPerBatch := 1

	targetCircuitAssetCounts := 0
	for _, v := range utils.AssetCountsTiers {
		if targetAssetCounts <= v {
			targetCircuitAssetCounts = v
			break
		}
	}
	emptyUserCircuit := NewBatchCreateUserCircuit(uint32(targetCircuitAssetCounts), uint32(totalAssetsCount), uint32(userOpsPerBatch))
	s := time.Now()
	var builder frontend.NewBuilder
	if provingSystem == "plonk" {
		builder = scs.NewBuilder
	} else if provingSystem == "groth16" {
		builder = r1cs.NewBuilder
	} else {
		return nil, nil, fmt.Errorf("invalid proving system")
	}
	oR1cs, err := frontend.Compile(ecc.BN254.ScalarField(), builder, emptyUserCircuit, frontend.IgnoreUnconstrainedInputs())
	if err != nil {
		return nil, nil, err
	}
	et := time.Now()
	fmt.Println("compile time is ", et.Sub(s))
	fmt.Println("batch create user constraints number is ", oR1cs.GetNbConstraints())

	userCircuit := ConstructValidBatch(targetAssetCounts, totalAssetsCount, userOpsPerBatch)

	witness, e := frontend.NewWitness(userCircuit, ecc.BN254.ScalarField())
	if witness == nil {
		return nil, nil, e
	}
	return oR1cs, witness, nil
}

func TestBatchCreateUserCircuit(t *testing.T) {
	oR1cs, witness, err := ConstructR1csAndWitness("groth16")
	if err != nil {
		t.Fatal(err)
	}
	err = oR1cs.IsSolved(witness)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBatchCreateUserCircuitFromKeySetup(t *testing.T) {
	oR1cs, witness, err := ConstructR1csAndWitness("groth16")
	if err != nil {
		t.Fatal(err)
	}
	pk, vk, err := groth16.Setup(oR1cs)
	if err != nil {
		panic(err)
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

func TestBatchCreateUserCircuitFromPlonkKeySetup(t *testing.T) {
	oScs, witness, err := ConstructR1csAndWitness("plonk")
	if err != nil {
		t.Fatal(err)
	}
	srs, srsLang, err := unsafekzg.NewSRS(oScs)
	if err != nil {
		panic(err)
	}
	pk, vk, err := plonk.Setup(oScs, srs, srsLang)
	if err != nil {
		panic(err)
	}
	publicWitness, err := witness.Public()
	if err != nil {
		panic(err)
	} else {
		fmt.Println("public witness")
	}
	startTime := time.Now()
	proof, err := plonk.Prove(oScs, pk, witness)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("proof")
	}
	endTime := time.Now()
	fmt.Println("prove time is ", endTime.Sub(startTime))
	err = plonk.Verify(proof, vk, publicWitness)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("verify")
	}
}

func TestBatchCreateUserCircuitFromKeyFiles(t *testing.T) {
	oR1cs, witness, err := ConstructR1csAndWitness("groth16")
	if err != nil {
		t.Fatal(err)
	}
	s := time.Now()
	r1csFromFile, err := os.ReadFile("../src/keygen/zkpor50_1.r1cs")
	if err != nil {
		panic(err)
	}
	buf := bytes.NewBuffer(r1csFromFile)
	newR1CS := groth16.NewCS(ecc.BN254)
	_, _ = newR1CS.ReadFrom(buf)
	et := time.Now()
	fmt.Println("read r1cs time is ", et.Sub(s))

	s = time.Now()
	pkFromFile, err := os.ReadFile("../src/keygen/zkpor50_1.pk")
	if err != nil {
		panic(err)
	}
	buf = bytes.NewBuffer(pkFromFile)
	pk := groth16.NewProvingKey(ecc.BN254)
	pk.UnsafeReadFrom(buf)
	et = time.Now()
	fmt.Println("read pk time is ", et.Sub(s))

	s = time.Now()
	vkFromFile, err := os.ReadFile("../src/keygen/zkpor50_1.vk")
	if err != nil {
		panic(err)
	}
	buf = bytes.NewBuffer(vkFromFile)
	vk := groth16.NewVerifyingKey(ecc.BN254)
	_, _ = vk.ReadFrom(buf)
	et = time.Now()
	fmt.Println("read vk time is ", et.Sub(s))

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

func TestBatchCreateUserCircuitFromWitnessFile(t *testing.T) {
	targetAssetCounts := 30
	totalAssetsCount := 500
	userOpsPerBatch := 1
	targetCircuitAssetCounts := 0
	for _, v := range utils.AssetCountsTiers {
		if targetAssetCounts <= v {
			targetCircuitAssetCounts = v
			break
		}
	}
	userCircuit := NewBatchCreateUserCircuit(uint32(targetCircuitAssetCounts), uint32(totalAssetsCount), uint32(userOpsPerBatch))
	s := time.Now()
	oR1cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, userCircuit, frontend.IgnoreUnconstrainedInputs())
	if err != nil {
		t.Fatal(err)
	}
	et := time.Now()
	fmt.Println("compile time is ", et.Sub(s))
	fmt.Println("batch create user constraints number is ", oR1cs.GetNbConstraints())

	// the witness.log can be generated by dbtool query_witness_data subcommand
	userCircuit = ConstructBatchFromFile("witness.log")
	solver.RegisterHint(IntegerDivision)
	witness, e := frontend.NewWitness(userCircuit, ecc.BN254.ScalarField())
	if witness == nil {
		t.Fatal(e)
		t.Fatal("witness is nil")
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

func ConstructBatchFromFile(fileName string) (witness *BatchCreateUserCircuit) {
	witnessFile, err := os.ReadFile(fileName)
	if err != nil {
		panic(err.Error())
	}
	witnessData := make([]byte, hex.DecodedLen(len(witnessFile)))
	n, err := hex.Decode(witnessData, witnessFile)
	if err != nil {
		panic(err.Error())
	}
	witnessData = witnessData[:n]

	witnessForCircuit := utils.DecodeBatchWitness(string(witnessData[:]))
	circuitWitness, _ := SetBatchCreateUserCircuitWitness(witnessForCircuit)
	return circuitWitness
}

func ConstructValidBatch(assetsCount int, totalAssetsCount int, userOpsPerBatch int) (witness *BatchCreateUserCircuit) {
	accountTree, err := utils.NewAccountTree("memory", "")
	if err != nil {
		panic(err.Error())
	}
	beforeAccountRoot := accountTree.Root()
	// construct cex assets
	cexAssets := make([]utils.CexAssetInfo, totalAssetsCount)
	for i := 0; i < totalAssetsCount; i++ {
		u := utils.CexAssetInfo{
			BasePrice: 1,
			Index:     uint32(i),
		}
		avgRatio := 100 / utils.TierCount
		for j := 0; j < utils.TierCount; j++ {
			u.LoanRatios[j] = utils.TierRatio{
				BoundaryValue:    new(big.Int).SetInt64(int64(100 * (j + 1))),
				Ratio:            uint8(100 - avgRatio*j),
				PrecomputedValue: new(big.Int).SetInt64(0),
			}
			u.MarginRatios[j] = utils.TierRatio{
				BoundaryValue:    new(big.Int).SetInt64(int64(100 * (j + 1))),
				Ratio:            uint8(100 - avgRatio*j),
				PrecomputedValue: new(big.Int).SetInt64(0),
			}
			u.PortfolioMarginRatios[j] = utils.TierRatio{
				BoundaryValue:    new(big.Int).SetInt64(int64(100 * (j + 1))),
				Ratio:            uint8(100 - avgRatio*j),
				PrecomputedValue: new(big.Int).SetInt64(0),
			}
		}
		utils.CalculatePrecomputedValue(u.LoanRatios[:])
		utils.CalculatePrecomputedValue(u.MarginRatios[:])
		utils.CalculatePrecomputedValue(u.PortfolioMarginRatios[:])
		cexAssets[i] = u
	}

	gap := totalAssetsCount / assetsCount
	// construct accounts
	accounts := make([]utils.AccountInfo, userOpsPerBatch)
	batchCreateUserWit := &utils.BatchCreateUserWitness{
		BeforeAccountTreeRoot: beforeAccountRoot,
		BeforeCexAssets:       make([]utils.CexAssetInfo, totalAssetsCount),
		CreateUserOps:         make([]utils.CreateUserOperation, userOpsPerBatch),
	}
	for i := 0; i < totalAssetsCount; i++ {
		batchCreateUserWit.BeforeCexAssets[i] = cexAssets[i]
	}
	batchCreateUserWit.BeforeCEXAssetsCommitment = utils.ComputeCexAssetsCommitment(batchCreateUserWit.BeforeCexAssets)

	for i := 0; i < len(accounts); i++ {
		accounts[i] = utils.AccountInfo{
			AccountIndex: uint32(i * 10),
			AccountId:    make([]byte, 32),
		}
		rand.Read(accounts[i].AccountId)
		accounts[i].AccountId = new(fr.Element).SetBytes(accounts[i].AccountId).Marshal()
		accounts[i].Assets = make([]utils.AccountAsset, assetsCount)
		totalEquity := new(big.Int).SetInt64(0)
		totalDebt := new(big.Int).SetInt64(0)
		totalCollateral := new(big.Int).SetInt64(0)

		for j := 0; j < len(accounts[i].Assets); j++ {
			accounts[i].Assets[j].Index = uint16(gap * j)
			assetPrice := new(big.Int).SetUint64(cexAssets[accounts[i].Assets[j].Index].BasePrice)
			accounts[i].Assets[j].Loan = uint64(rand.Intn(1000)) + 1
			accounts[i].Assets[j].Margin = uint64(rand.Intn(1000)) + 1
			accounts[i].Assets[j].PortfolioMargin = uint64(rand.Intn(1000)) + 1
			totalValue := accounts[i].Assets[j].Loan + accounts[i].Assets[j].Margin + accounts[i].Assets[j].PortfolioMargin
			collateralValue := utils.CalculateAssetValueForCollateral(accounts[i].Assets[j].Loan,
				accounts[i].Assets[j].Margin,
				accounts[i].Assets[j].PortfolioMargin,
				&cexAssets[accounts[i].Assets[j].Index])
			totalCollateral.Add(totalCollateral, collateralValue)
			collateralValue.Div(collateralValue, assetPrice)
			accounts[i].Assets[j].Debt = uint64(rand.Intn(int(collateralValue.Int64()))) + 1
			accounts[i].Assets[j].Equity = uint64(rand.Intn(1000)) + totalValue
			debtBigInt := new(big.Int).SetUint64(accounts[i].Assets[j].Debt)
			equityBigInt := new(big.Int).SetUint64(accounts[i].Assets[j].Equity)
			debtBigInt.Mul(debtBigInt, assetPrice)
			totalDebt.Add(totalDebt, debtBigInt)
			equityBigInt.Mul(equityBigInt, assetPrice)
			totalEquity.Add(totalEquity, equityBigInt)
			// update cexAssets
			cexAssets[accounts[i].Assets[j].Index].TotalEquity += accounts[i].Assets[j].Equity
			cexAssets[accounts[i].Assets[j].Index].TotalDebt += accounts[i].Assets[j].Debt
			cexAssets[accounts[i].Assets[j].Index].LoanCollateral += accounts[i].Assets[j].Loan
			cexAssets[accounts[i].Assets[j].Index].MarginCollateral += accounts[i].Assets[j].Margin
			cexAssets[accounts[i].Assets[j].Index].PortfolioMarginCollateral += accounts[i].Assets[j].PortfolioMargin
		}
		accounts[i].TotalEquity = totalEquity
		accounts[i].TotalDebt = totalDebt
		accounts[i].TotalCollateral = totalCollateral
		poseidonHasher := poseidon.NewPoseidon()
		accountBeforeRoot := accountTree.Root()
		accountProof, err := accountTree.GetProof(uint64(accounts[i].AccountIndex))
		if err != nil {
			panic(err.Error())
		}
		accountTree.Set(uint64(accounts[i].AccountIndex), utils.AccountInfoToHash(&accounts[i], &poseidonHasher))
		accountAfterRoot := accountTree.Root()
		batchCreateUserWit.CreateUserOps[i] = utils.CreateUserOperation{
			BeforeAccountTreeRoot: accountBeforeRoot,
			AfterAccountTreeRoot:  accountAfterRoot,
			Assets:                accounts[i].Assets,
			AccountIndex:          accounts[i].AccountIndex,
			AccountIdHash:         accounts[i].AccountId,
		}
		copy(batchCreateUserWit.CreateUserOps[i].AccountProof[:], accountProof[:])

	}

	batchCreateUserWit.AfterAccountTreeRoot = accountTree.Root()
	batchCreateUserWit.AfterCEXAssetsCommitment = utils.ComputeCexAssetsCommitment(cexAssets)
	batchCreateUserWit.BatchCommitment = poseidon.PoseidonBytes(batchCreateUserWit.BeforeAccountTreeRoot,
		batchCreateUserWit.AfterAccountTreeRoot,
		batchCreateUserWit.BeforeCEXAssetsCommitment,
		batchCreateUserWit.AfterCEXAssetsCommitment)
	var serializeBuf bytes.Buffer
	enc := gob.NewEncoder(&serializeBuf)
	err = enc.Encode(batchCreateUserWit)
	if err != nil {
		panic(err.Error())
	}
	buf := serializeBuf.Bytes()
	compressedBuf := s2.Encode(nil, buf)
	witnessDataStr := base64.StdEncoding.EncodeToString(compressedBuf)
	witnessForCircuit := utils.DecodeBatchWitness(witnessDataStr)
	circuitWitness, _ := SetBatchCreateUserCircuitWitness(witnessForCircuit)
	return circuitWitness
}

func TestSetBatchCreateUserCircuitWitness(t *testing.T) {
	targetAssetCounts := 50
	userOpsPerBatch := 1
	circuitWitness := ConstructValidBatch(targetAssetCounts, utils.AssetCounts, userOpsPerBatch)
	for i := 0; i < len(circuitWitness.CreateUserOps); i++ {
		if len(circuitWitness.CreateUserOps[i].Assets) != targetAssetCounts {
			t.Fatal("asset counts not match")
		}
	}
	fmt.Println("assets info ", circuitWitness.CreateUserOps[0].Assets[0].LoanCollateralIndex)
	fmt.Println("assets info ", circuitWitness.CreateUserOps[0].Assets[0].MarginCollateralIndex)
	fmt.Println("assets info ", circuitWitness.CreateUserOps[0].Assets[0].PortfolioMarginCollateralIndex)
}

type PoseidonCircuit struct {
	Vs []Variable
}

func (c PoseidonCircuit) Define(api API) error {
	v := poseidon2.Poseidon(api, c.Vs...)
	api.AssertIsEqual(v, c.Vs[0])
	return nil
}

func TestPoseidon(t *testing.T) {
	circuit := PoseidonCircuit{
		Vs: make([]frontend.Variable, 37),
	}

	r1cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit, frontend.IgnoreUnconstrainedInputs())
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("poseidon constraints number is ", r1cs.GetNbConstraints())
}
