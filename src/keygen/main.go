package main

import (
	"fmt"
	"os"

	"github.com/binance/zkmerkle-proof-of-solvency/circuit"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/consensys/gnark-crypto/ecc"

	"github.com/consensys/gnark/backend/groth16"
	"runtime"
	"time"

	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"strconv"
)

func main() {
	go func() {
		for {
			time.Sleep(time.Second * 10)
			runtime.GC()
		}
	}()
	for k, v := range utils.BatchCreateUserOpsCountsTiers {
		circuit := circuit.NewBatchCreateUserCircuit(uint32(k), utils.AssetCounts, uint32(v))
		startTime := time.Now()
		oR1cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, circuit, frontend.IgnoreUnconstrainedInputs())
		if err != nil {
			panic(err)
		}
		endTime := time.Now()
		fmt.Println("R1CS generation tims is ", endTime.Sub(startTime))
		fmt.Println("batch create user constraints number is ", oR1cs.GetNbConstraints())
		zkKeyName := "zkpor" + strconv.FormatInt(int64(k), 10) + "_" + strconv.FormatInt(int64(v), 10)
		pkFile, err := os.Create(zkKeyName + ".pk")
		if err != nil {
			panic(err)
		}
		pk, vk, err := groth16.Setup(oR1cs)
		if err != nil {
			panic(err)
		}
		n, err := pk.WriteTo(pkFile)
		if err != nil {
			panic(err)
		}
		fmt.Println("pk size is ", n)
		vkFile, err := os.Create(zkKeyName + ".vk")
		if err != nil {
			panic(err)
		}
		n, err = vk.WriteTo(vkFile)
		if err != nil {
			panic(err)
		}
		fmt.Println("pk size is ", n)

		r1csFile, _ := os.Create(zkKeyName + ".r1cs")
		n, err = oR1cs.WriteTo(r1csFile)
		if err != nil {
			panic(err)
		}
		fmt.Println("r1cs size is ", n)
	}
}
