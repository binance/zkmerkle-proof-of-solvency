package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"runtime"
	"sync"

	"github.com/binance/zkmerkle-proof-of-solvency/circuit"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/binance/zkmerkle-proof-of-solvency/src/verifier/config"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/gocarina/gocsv"
)

func LoadVerifyingKey(vkFileName string) (groth16.VerifyingKey, error) {
	vkFile, err := os.ReadFile(vkFileName)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(vkFile)
	vk := groth16.NewVerifyingKey(ecc.BN254)
	_, err = vk.ReadFrom(buf)
	if err != nil {
		return nil, err
	}
	return vk, nil
}


func main() {
	userFlag := flag.Bool("user", false, "flag which indicates user proof verification")
	hashFlag := flag.Bool("hash", false, "flag which indicates hash command")
	flag.Parse()
	if *userFlag {
		userConfig := &config.UserConfig{}
		content, err := ioutil.ReadFile("config/user_config.json")
		if err != nil {
			panic(err.Error())
		}
		err = json.Unmarshal(content, userConfig)
		if err != nil {
			panic(err.Error())
		}
		root, err := hex.DecodeString(userConfig.Root)
		if err != nil || len(root) != 32 {
			panic("invalid account tree root")
		}

		var proof [][]byte
		for i := 0; i < len(userConfig.Proof); i++ {
			p, err := base64.StdEncoding.DecodeString(userConfig.Proof[i])
			if err != nil || len(p) != 32 {
				panic("invalid proof")
			}
			proof = append(proof, p)
		}

		// padding user assets
		hasher := poseidon.NewPoseidon()
		assetCommitment := utils.ComputeUserAssetsCommitment(&hasher, userConfig.Assets)
		hasher.Reset()
		// compute new account leaf node hash
		accountIdHash, err := hex.DecodeString(userConfig.AccountIdHash)
		if err != nil || len(accountIdHash) != 32 {
			panic("the AccountIdHash is invalid")
		}
		accountHash := poseidon.PoseidonBytes(accountIdHash, userConfig.TotalEquity.Bytes(), userConfig.TotalDebt.Bytes(), userConfig.TotalCollateral.Bytes(), assetCommitment)
		fmt.Println("user merkle leave hash base64 encode: ", base64.StdEncoding.EncodeToString(accountHash))
		fmt.Printf("user merkle leave hash hex encode: %x\n", accountHash)
		verifyFlag := utils.VerifyMerkleProof(root, userConfig.AccountIndex, proof, accountHash)
		if verifyFlag {
			fmt.Println("verify pass!!!")
		} else {
			fmt.Println("verify failed...")
		}
	} else if (*hashFlag) {
		args := flag.Args()
		if len(args) != 2 {
			panic("invalid hash command, it needs two arguments")
		}
		hasher := poseidon.NewPoseidon()
		p0, err := base64.StdEncoding.DecodeString(args[0])
		if err != nil {
			panic("invalid hash command, the first argument is not base64 encoded")
		}
		p1, err := base64.StdEncoding.DecodeString(args[1])
		if err != nil {
			panic("invalid hash command, the second argument is not base64 encoded")
		}
		hasher.Write(p0)
		hasher.Write(p1)
		res := hasher.Sum(nil)
		resBase64 := base64.StdEncoding.EncodeToString(res)
		fmt.Printf("hash result base64 encode: %s\n", resBase64)
		fmt.Printf("hash result hex encode: %x\n", res)
	} else {
		verifierConfig := &config.Config{}
		content, err := ioutil.ReadFile("config/config.json")
		if err != nil {
			panic(err.Error())
		}
		err = json.Unmarshal(content, verifierConfig)
		if err != nil {
			panic(err.Error())
		}

		f, err := os.Open(verifierConfig.ProofTable)
		if err != nil {
			panic(err.Error())
		}
		defer f.Close()
		// index 4: proof_info, index 5: cex_asset_list_commitments
		// index 6: account_tree_roots, index 7: batch_commitment
		// index 8: batch_number
		type Proof struct {
			BatchNumber        int64    `csv:"batch_number"`
			ZkProof            string   `csv:"proof_info"`
			CexAssetCommitment []string `csv:"cex_asset_list_commitments"`
			AccountTreeRoots   []string `csv:"account_tree_roots"`
			BatchCommitment    string   `csv:"batch_commitment"`
			AssetsCount        int      `csv:"assets_count"`
		}
		tmpProofs := []*Proof{}

		err = gocsv.UnmarshalFile(f, &tmpProofs)
		if err != nil {
			panic(err.Error())
		}

		proofs := make([]Proof, len(tmpProofs))
		for i := 0; i < len(tmpProofs); i++ {
			proofs[tmpProofs[i].BatchNumber] = *tmpProofs[i]
		}

		prevCexAssetListCommitments := make([][]byte, 2)
		prevAccountTreeRoots := make([][]byte, 2)
		// depth-28 empty account tree root
		emptyAccountTreeRoot, err := hex.DecodeString("08696bfcb563a2ee4dde9e1dbd34f68d3f4643df6e3709cdb1855c9f886240c7")
		if err != nil {
			fmt.Println("wrong empty empty account tree root")
			return
		}
		prevAccountTreeRoots[1] = emptyAccountTreeRoot
		// according to asset price info to compute
		cexAssetsInfo := make([]utils.CexAssetInfo, len(verifierConfig.CexAssetsInfo))
		for i := 0; i < len(verifierConfig.CexAssetsInfo); i++ {
			cexAssetsInfo[verifierConfig.CexAssetsInfo[i].Index] = verifierConfig.CexAssetsInfo[i]
			if verifierConfig.CexAssetsInfo[i].TotalEquity < verifierConfig.CexAssetsInfo[i].TotalDebt {
				fmt.Printf("%s asset equity %d less then debt %d\n", verifierConfig.CexAssetsInfo[i].Symbol, verifierConfig.CexAssetsInfo[i].TotalEquity, verifierConfig.CexAssetsInfo[i].TotalDebt)
				panic("invalid cex asset info")
			}
		}
		emptyCexAssetsInfo := make([]utils.CexAssetInfo, len(cexAssetsInfo))
		copy(emptyCexAssetsInfo, cexAssetsInfo)
		for i := 0; i < len(emptyCexAssetsInfo); i++ {
			emptyCexAssetsInfo[i].TotalDebt = 0
			emptyCexAssetsInfo[i].TotalEquity = 0
			emptyCexAssetsInfo[i].LoanCollateral = 0
			emptyCexAssetsInfo[i].MarginCollateral = 0
			emptyCexAssetsInfo[i].PortfolioMarginCollateral = 0
		}
		emptyCexAssetListCommitment := utils.ComputeCexAssetsCommitment(emptyCexAssetsInfo)
		expectFinalCexAssetsInfoComm := utils.ComputeCexAssetsCommitment(cexAssetsInfo)
		prevCexAssetListCommitments[1] = emptyCexAssetListCommitment
		var finalCexAssetsInfoComm []byte
		var accountTreeRoot []byte

		workersNum := 16
		if runtime.NumCPU() > workersNum {
			workersNum = runtime.NumCPU()
		}
		averageProofCount := (len(proofs) + workersNum - 1) / workersNum
		
		type ProofMetaData struct {
			accountTreeRoots [][]byte
			cexAssetListCommitments [][]byte
		}
		type SafeProofMap struct {
			sync.Mutex
			proofMap map[int]ProofMetaData
		}
		safeProofMap := &SafeProofMap{proofMap: make(map[int]ProofMetaData)}
		var wg sync.WaitGroup
		for i := 0; i < workersNum; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				var vk groth16.VerifyingKey
				currentAssetCountsTier := 0
				startIndex := index * averageProofCount
				endIndex := (index + 1) * averageProofCount
				if endIndex > len(proofs) {
					endIndex = len(proofs)
				}
				for j := startIndex; j < endIndex; j++ {
					batchNumber := int(proofs[j].BatchNumber)
					// first deserialize proof
					proof := groth16.NewProof(ecc.BN254)
					var bufRaw bytes.Buffer
					proofRaw, err := base64.StdEncoding.DecodeString(proofs[j].ZkProof)
					if err != nil {
						fmt.Println("decode proof failed:", batchNumber)
						panic("verify proof " + strconv.Itoa(batchNumber) + " failed")
					}
					bufRaw.Write(proofRaw)
					proof.ReadFrom(&bufRaw)
					// deserialize cex asset list commitment and account tree root
					cexAssetListCommitments := make([][]byte, 2)
					accountTreeRoots := make([][]byte, 2)

					for p := 0; p < len(proofs[j].CexAssetCommitment); p++ {
						cexAssetListCommitments[p], err = base64.StdEncoding.DecodeString(proofs[j].CexAssetCommitment[p])
						if err != nil {
							fmt.Println("decode cex asset commitment failed")
							panic(err.Error())
						}
					}
					for p := 0; p < len(proofs[j].AccountTreeRoots); p++ {
						accountTreeRoots[p], err = base64.StdEncoding.DecodeString(proofs[j].AccountTreeRoots[p])
						if err != nil {
							fmt.Println("decode account tree root failed")
							panic(err.Error())
						}
					}
					// verify the public input is correctly computed by cex asset list and account tree root
					poseidonHasher := poseidon.NewPoseidon()
					poseidonHasher.Write(accountTreeRoots[0])
					poseidonHasher.Write(accountTreeRoots[1])
					poseidonHasher.Write(cexAssetListCommitments[0])
					poseidonHasher.Write(cexAssetListCommitments[1])
					expectHash := poseidonHasher.Sum(nil)
					actualHash, err := base64.StdEncoding.DecodeString(proofs[j].BatchCommitment)
					if err != nil {
						fmt.Println("decode batch commitment failed", batchNumber)
						panic("verify proof " + strconv.Itoa(batchNumber) + " failed")
					}
					if string(expectHash) != string(actualHash) {
						fmt.Println("public input verify failed ", batchNumber)
						fmt.Printf("%x:%x\n", expectHash, actualHash)
						panic("verify proof " + strconv.Itoa(batchNumber) + " failed")
					}
					safeProofMap.Lock()
					safeProofMap.proofMap[int(batchNumber)] = ProofMetaData{accountTreeRoots: accountTreeRoots, cexAssetListCommitments: cexAssetListCommitments}
					safeProofMap.Unlock()
					verifyWitness := circuit.NewVerifyBatchCreateUserCircuit(actualHash)
					vWitness, err := frontend.NewWitness(verifyWitness, ecc.BN254.ScalarField(), frontend.PublicOnly())
					if err != nil {
						panic(err.Error())
					}
					if proofs[j].AssetsCount != currentAssetCountsTier {
						index := -1
						for p := 0; p < len(verifierConfig.AssetsCountTiers); p++ {
							if verifierConfig.AssetsCountTiers[p] == proofs[j].AssetsCount {
								index = p
								break
							}
						}
						if index == -1 {
							panic("invalid asset counts tier")
						}
						vk, err = LoadVerifyingKey(verifierConfig.ZkKeyName[index] + ".vk")
						if err != nil {
							panic(err.Error())
						}
						currentAssetCountsTier = proofs[j].AssetsCount
					}
					err = groth16.Verify(proof, vk, vWitness)
					if err != nil {
						fmt.Println("proof verify failed:", batchNumber, err.Error())
						return
					} else {
						fmt.Println("proof verify success", batchNumber)
					}
				}

			}(i)
		}

		wg.Wait()
		for batchNumber := 0; batchNumber < len(proofs); batchNumber++ {
			proofData, ok := safeProofMap.proofMap[batchNumber]
			if !ok {
				panic("proof data not found: " + strconv.Itoa(batchNumber))
			}
			if string(proofData.accountTreeRoots[0]) != string(prevAccountTreeRoots[1]) {
				panic("account tree root not match: " + strconv.Itoa(batchNumber))
			}
			if string(proofData.cexAssetListCommitments[0]) != string(prevCexAssetListCommitments[1]) {
				panic("cex asset list commitment not match: " + strconv.Itoa(batchNumber))
			}
			prevAccountTreeRoots = proofData.accountTreeRoots
			prevCexAssetListCommitments = proofData.cexAssetListCommitments
			accountTreeRoot = proofData.accountTreeRoots[1]
			finalCexAssetsInfoComm = proofData.cexAssetListCommitments[1]
		}

		if string(finalCexAssetsInfoComm) != string(expectFinalCexAssetsInfoComm) {
			panic("Final Cex Assets Info Not Match")
		}
		fmt.Printf("account merkle tree root is %x\n", accountTreeRoot)
		fmt.Println("All proofs verify passed!!!")
	}
}
