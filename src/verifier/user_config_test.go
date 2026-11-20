package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/binance/zkmerkle-proof-of-solvency/src/verifier/config"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
)

// TestSampleUserConfigVerifies keeps the sample config/user_config.json (also
// shown in the README) consistent with the leaf hash and Merkle proof checks
// that `verifier -user` performs.
func TestSampleUserConfigVerifies(t *testing.T) {
	content, err := os.ReadFile("config/user_config.json")
	if err != nil {
		t.Fatal(err)
	}
	userConfig := &config.UserConfig{}
	if err := json.Unmarshal(content, userConfig); err != nil {
		t.Fatal(err)
	}
	root, err := hex.DecodeString(userConfig.Root)
	if err != nil || len(root) != 32 {
		t.Fatal("invalid account tree root")
	}
	var proof [][]byte
	for _, s := range userConfig.Proof {
		p, err := base64.StdEncoding.DecodeString(s)
		if err != nil || len(p) != 32 {
			t.Fatal("invalid proof")
		}
		proof = append(proof, p)
	}
	accountIdHash, err := hex.DecodeString(userConfig.AccountIdHash)
	if err != nil || len(accountIdHash) != 32 {
		t.Fatal("invalid AccountIdHash")
	}
	hasher := poseidon.NewPoseidon()
	assetCommitment := utils.ComputeUserAssetsCommitment(&hasher, userConfig.Assets)
	leaf := poseidon.PoseidonBytes(accountIdHash, userConfig.TotalEquity.Bytes(), userConfig.TotalDebt.Bytes(), userConfig.TotalCollateral.Bytes(), assetCommitment)

	if !utils.VerifyMerkleProof(root, userConfig.AccountIndex, proof, leaf) {
		t.Fatal("sample user_config.json does not verify")
	}
	// a changed balance must not verify against the same proof
	tampered := poseidon.PoseidonBytes(accountIdHash, userConfig.TotalEquity.Bytes(), []byte{1}, userConfig.TotalCollateral.Bytes(), assetCommitment)
	if utils.VerifyMerkleProof(root, userConfig.AccountIndex, proof, tampered) {
		t.Fatal("tampered leaf unexpectedly verified")
	}
}
