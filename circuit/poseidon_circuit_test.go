package circuit

import (
	"bytes"
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	poseidon2 "github.com/consensys/gnark/std/hash/poseidon"
)

type PoseidonCircuit struct {
	Vs []Variable
}

func (c PoseidonCircuit) Define(api API) error {
	v := poseidon2.Poseidon(api, c.Vs[0], c.Vs[1])
	for i := 2; i < len(c.Vs); i++ {
		v = poseidon2.Poseidon(api, v, c.Vs[i])
	}
	// api.AssertIsEqual(v, c.Vs[0])
	return nil
}

// printMemStats prints current memory statistics
func printMemStats(label string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("[%s] Alloc: %d MB, TotalAlloc: %d MB, Sys: %d MB, NumGC: %d\n",
		label,
		m.Alloc/1024/1024,
		m.TotalAlloc/1024/1024,
		m.Sys/1024/1024,
		m.NumGC)
}

func TestPoseidon(t *testing.T) {
	// Print CPU info
	fmt.Println("=== System Info ===")
	fmt.Println("NumCPU:", runtime.NumCPU())
	fmt.Println("GOMAXPROCS:", runtime.GOMAXPROCS(0))

	// Initial memory stats
	runtime.GC()
	printMemStats("Initial")

	hashCounts := 200
	circuit := PoseidonCircuit{
		Vs: make([]frontend.Variable, hashCounts),
	}

	r1cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit, frontend.IgnoreUnconstrainedInputs())
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("poseidon constraints number is ", r1cs.GetNbConstraints())
	printMemStats("After Compile")

	// Generate witness with random values
	witnessCircuit := PoseidonCircuit{
		Vs: make([]frontend.Variable, hashCounts),
	}
	for i := 0; i < len(witnessCircuit.Vs); i++ {
		witnessCircuit.Vs[i] = rand.Int63()
	}

	// Create witness
	witness, err := frontend.NewWitness(&witnessCircuit, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatal(err)
	}
	printMemStats("After Witness Creation")

	// Key generation (setup)
	fmt.Println("Starting key generation...")
	startTime := time.Now()
	pk, vk, err := groth16.Setup(r1cs)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Key generation time:", time.Since(startTime))
	printMemStats("After Key Generation")

	// Get public witness
	publicWitness, err := witness.Public()
	if err != nil {
		t.Fatal(err)
	}

	// Prove
	fmt.Println("Starting proof generation...")
	startTime = time.Now()
	proof, err := groth16.Prove(r1cs, pk, witness)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Proof generation time:", time.Since(startTime))
	printMemStats("After Proof Generation")

	// Output proof size
	var proofBuf bytes.Buffer
	_, err = proof.WriteTo(&proofBuf)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Proof size:", proofBuf.Len(), "bytes")

	// Verify
	fmt.Println("Starting verification...")
	startTime = time.Now()
	err = groth16.Verify(proof, vk, publicWitness)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Verification time:", time.Since(startTime))
	printMemStats("After Verification")

	// Final statistics
	fmt.Println("\n=== Final Statistics ===")
	var finalStats runtime.MemStats
	runtime.ReadMemStats(&finalStats)
	fmt.Printf("Peak Sys Memory: %d MB\n", finalStats.Sys/1024/1024)
	fmt.Printf("Total Allocations: %d MB\n", finalStats.TotalAlloc/1024/1024)
	fmt.Printf("Total GC Cycles: %d\n", finalStats.NumGC)
	fmt.Println("Verification successful!")
}
