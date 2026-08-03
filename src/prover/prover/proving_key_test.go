package prover

import (
	"bytes"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	curve "github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark/backend/groth16"
	groth16bn254 "github.com/consensys/gnark/backend/groth16/bn254"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

type provingKeyTestCircuit struct {
	Value frontend.Variable
}

func (c *provingKeyTestCircuit) Define(api frontend.API) error {
	api.AssertIsEqual(c.Value, 1)
	return nil
}

func TestReadProvingKeyRejectsPointsOutsideSubgroup(t *testing.T) {
	constraintSystem, err := frontend.Compile(
		ecc.BN254.ScalarField(),
		r1cs.NewBuilder,
		&provingKeyTestCircuit{},
	)
	if err != nil {
		t.Fatal(err)
	}

	provingKey, _, err := groth16.Setup(constraintSystem)
	if err != nil {
		t.Fatal(err)
	}
	typedProvingKey, ok := provingKey.(*groth16bn254.ProvingKey)
	if !ok {
		t.Fatalf("unexpected proving key type %T", provingKey)
	}

	var validEncoding bytes.Buffer
	if _, err := typedProvingKey.WriteTo(&validEncoding); err != nil {
		t.Fatal(err)
	}
	loadedProvingKey, bytesRead, err := readProvingKey(bytes.NewReader(validEncoding.Bytes()))
	if err != nil {
		t.Fatalf("expected a valid proving key to be accepted: %v", err)
	}
	if loadedProvingKey == nil {
		t.Fatal("expected a valid proving key to be returned")
	}
	if bytesRead != int64(validEncoding.Len()) {
		t.Fatalf("read %d bytes from a %d-byte proving key", bytesRead, validEncoding.Len())
	}

	var mapInput curve.G2Affine
	foundInvalidPoint := false
	for candidateIndex := uint64(1); candidateIndex <= 100; candidateIndex++ {
		mapInput.X.A0.SetUint64(candidateIndex)
		candidate := curve.MapToCurve2(&mapInput.X)
		if !candidate.IsInSubGroup() {
			typedProvingKey.G2.Beta = candidate
			foundInvalidPoint = true
			break
		}
	}
	if !foundInvalidPoint {
		t.Fatal("failed to construct an out-of-subgroup test point")
	}

	var encoded bytes.Buffer
	if _, err := typedProvingKey.WriteTo(&encoded); err != nil {
		t.Fatal(err)
	}

	unsafeProvingKey := groth16.NewProvingKey(ecc.BN254)
	if _, err := unsafeProvingKey.UnsafeReadFrom(bytes.NewReader(encoded.Bytes())); err != nil {
		t.Fatalf("test fixture must be accepted without subgroup checks: %v", err)
	}

	loadedProvingKey, _, err = readProvingKey(bytes.NewReader(encoded.Bytes()))
	if err == nil {
		t.Fatal("expected proving key with an out-of-subgroup point to be rejected")
	}
	if loadedProvingKey != nil {
		t.Fatal("expected a rejected proving key not to be returned")
	}
}
