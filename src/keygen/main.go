package main

import (
	"fmt"
	"github.com/binance/zkmerkle-proof-of-solvency/circuit"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"runtime"
	"strconv"
	"time"
	"os"
)

func main() {
	circuit := circuit.NewBatchCreateUserCircuit(utils.AssetCounts, utils.BatchCreateUserOpsCounts)
	oR1cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, circuit, frontend.IgnoreUnconstrainedInputs(), frontend.WithGKRBN(0))
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			select {
			case <-time.After(time.Second * 10):
				runtime.GC()
			}
		}
	}()
	fmt.Println(oR1cs.GetNbVariables())
	zkKeyName := "zkpor" + strconv.FormatInt(utils.BatchCreateUserOpsCounts, 10)
	fmt.Printf("Number of constraints: %d\n", oR1cs.GetNbConstraints())
	oR1cs.Lazify()
	fmt.Printf("After Lazify: Number of constraints: %d\n", oR1cs.GetNbConstraints())
	err = oR1cs.SplitDumpBinary(zkKeyName, utils.R1csBatchSize)
	if err != nil {
		panic(err)
	}
	oR1csFull := groth16.NewCS(ecc.BN254)
	oR1csFull.LoadFromSplitBinaryConcurrent(zkKeyName, oR1cs.GetNbR1C(), utils.R1csBatchSize, runtime.NumCPU())
	if err != nil {
		panic(err)
	}

	f, err := os.Create(zkKeyName + ".r1cslen")
	if err != nil {
		panic(err)
	}
	_, err = f.WriteString(fmt.Sprint(oR1csFull.GetNbR1C()))
	if err != nil {
		panic(err)
	}
	f.Close()
	err = groth16.SetupDumpKeys(oR1csFull, zkKeyName)
	if err != nil {
		panic(err)
	}
}
