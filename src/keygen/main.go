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
)

func main() {
	circuit := circuit.NewBatchCreateUserCircuit(utils.AssetCounts, utils.BatchCreateUserOpsCounts)
	oR1cs, err := frontend.Compile(ecc.BN254, r1cs.NewBuilder, circuit, frontend.IgnoreUnconstrainedInputs())
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
	err = groth16.SetupLazyWithDump(oR1cs, zkKeyName)
	if err != nil {
		panic(err)
	}
}
