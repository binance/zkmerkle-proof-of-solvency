package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils/merkletree"
	"github.com/binance/zkmerkle-proof-of-solvency/src/witness/config"
	"github.com/binance/zkmerkle-proof-of-solvency/src/witness/witness"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
)

func main() {
	remotePasswdConfig := flag.String("remote_password_config", "", "fetch password from aws secretsmanager")
	witnessDoneMarker := flag.String("witness_done_marker", "", "path to marker file created when witness generation completes")
	flag.Parse()
	witnessConfig := &config.Config{}
	content, err := ioutil.ReadFile("config/config.json")
	if err != nil {
		panic(err.Error())
	}
	err = json.Unmarshal(content, witnessConfig)
	if err != nil {
		panic(err.Error())
	}
	if *remotePasswdConfig != "" {
		s, err := utils.GetMysqlSource(witnessConfig.MysqlDataSource, *remotePasswdConfig)
		if err != nil {
			panic(err.Error())
		}
		witnessConfig.MysqlDataSource = s
	}

	accounts, cexAssetsInfo, err := utils.ParseUserDataSet(witnessConfig.UserDataFile)
	if err != nil {
		panic(err.Error())
	}

	totalAccountNum := 0
	for k, v := range accounts {
		totalAccountNum += len(v)
		fmt.Println("the asset counts of user is ", k, "total ops number is ", len(v))
	}
	fmt.Println("total account num before padding:", totalAccountNum)

	// Padding accounts to align with batch sizes.
	keys := make([]int, 0, len(accounts))
	for k := range accounts {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		accounts[k] = utils.PaddingAccounts(accounts[k], k)
	}

	// Assign AccountIndex sequentially (0, 1, 2, ...) in batch order
	// so that within each batch AccountIndex increments by 1,
	// and between consecutive batches the indices are contiguous.
	globalIndex := uint32(0)
	for _, k := range keys {
		for i := range accounts[k] {
			accounts[k][i].AccountIndex = globalIndex
			if len(accounts[k][i].AccountId) == 0 {
				var buf [4]byte
				binary.BigEndian.PutUint32(buf[:], globalIndex)
				h := sha256.Sum256(buf[:])
				accounts[k][i].AccountId = new(fr.Element).SetBytes(h[:]).Marshal()
			}
			globalIndex++
		}
	}

	// Compute total capacity (sum of all padded accounts).
	capacity := 0
	for _, v := range accounts {
		capacity += len(v)
	}
	fmt.Println("total capacity after padding:", capacity)

	// Create account tree with exact capacity.
	accountTree, err := utils.NewAccountTree(capacity)
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("account tree initialized")

	// Set all account leaves into the tree in parallel.
	buildAccountTree(accountTree, accounts, keys)
	fmt.Printf("account tree root is %x\n", accountTree.Root())

	witnessService := witness.NewWitness(accountTree, accounts, cexAssetsInfo, witnessConfig)
	userProofService := witness.NewUserProofService(accountTree, accounts, witnessService.GetDB(), witnessConfig.DbSuffix)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		witnessService.Run()
		fmt.Println("witness service run finished...")
		if *witnessDoneMarker != "" {
			f, err := os.Create(*witnessDoneMarker)
			if err != nil {
				fmt.Printf("failed to create witness done marker: %v\n", err)
			} else {
				f.Close()
			}
		}
	}()
	go func() {
		defer wg.Done()
		userProofService.Run()
	}()
	wg.Wait()
}

// buildAccountTree computes hashes for all accounts and sets them into the tree,
// then calls Build to compute internal nodes.
func buildAccountTree(tree *merkletree.FixedDepthMerkleTree, accounts map[int][]utils.AccountInfo, keys []int) {
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}

	total := 0
	for _, k := range keys {
		total += len(accounts[k])
	}
	fmt.Printf("buildAccountTree: total %d leaves to insert, workers=%d\n", total, workers)

	var inserted atomic.Int64
	startTime := time.Now()

	// Log progress periodically in the background.
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				cur := inserted.Load()
				elapsed := time.Since(startTime).Seconds()
				fmt.Printf("buildAccountTree: inserted %d / %d leaves (%.1f%%), elapsed %.1fs\n",
					cur, total, float64(cur)/float64(total)*100, elapsed)
			}
		}
	}()

	for _, k := range keys {
		tierAccounts := accounts[k]
		n := len(tierAccounts)

		chunkSize := (n + workers - 1) / workers
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			start := w * chunkSize
			end := start + chunkSize
			if end > n {
				end = n
			}
			if start >= end {
				break
			}
			wg.Add(1)
			go func(accs []utils.AccountInfo, start, end int) {
				defer wg.Done()
				poseidonHasher := poseidon.NewPoseidon()
				for i := start; i < end; i++ {
					accountHash := utils.AccountInfoToHash(&accs[i], &poseidonHasher)
					if err := tree.Set(accs[i].AccountIndex, accountHash); err != nil {
						panic(fmt.Sprintf("failed to set account %d: %v", accs[i].AccountIndex, err))
					}
				}
				inserted.Add(int64(end - start))
			}(tierAccounts, start, end)
		}
		wg.Wait()
	}
	close(done)

	fmt.Printf("buildAccountTree: all %d leaves inserted, elapsed %.1fs. Building tree...\n",
		inserted.Load(), time.Since(startTime).Seconds())
	tree.Build()
	fmt.Printf("buildAccountTree: tree built, total elapsed %.1fs\n", time.Since(startTime).Seconds())
}
