package witness

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils/merkletree"
	"gorm.io/gorm"
)

// UserProofService generates user proofs from the in-memory account tree
// and writes them to MySQL. It adapts the standalone userproof service
// (Mode 2 logic) to run within the witness package.
type UserProofService struct {
	accountTree    *merkletree.FixedDepthMerkleTree
	accounts       map[int][]utils.AccountInfo
	userProofModel UserProofModel
}

// NewUserProofService creates a new UserProofService.
func NewUserProofService(accountTree *merkletree.FixedDepthMerkleTree,
	accounts map[int][]utils.AccountInfo, db *gorm.DB, dbSuffix string) *UserProofService {
	return &UserProofService{
		accountTree:    accountTree,
		accounts:       accounts,
		userProofModel: NewUserProofModel(db, dbSuffix),
	}
}

// Run generates user proofs for all accounts and writes them to the database.
// It supports resume: if proofs already exist in the DB, it skips those accounts.
func (s *UserProofService) Run() {
	// Step 1: Create userproof table.
	err := s.userProofModel.CreateUserProofTable()
	if err != nil {
		panic("create userproof table failed: " + err.Error())
	}

	// Step 2: Query latest account index for resume support (with retry on timeout).
	// Uses indexed ORDER BY ... LIMIT 1 instead of COUNT(*), which requires a
	// full index scan on large InnoDB tables.
	var currentAccountCounts int
	for {
		latestIdx, queryErr := s.userProofModel.GetLatestAccountIndex()
		if queryErr == utils.DbErrQueryInterrupted || queryErr == utils.DbErrQueryTimeout {
			fmt.Println("get latest account index timeout, retry...:", queryErr.Error())
			time.Sleep(1 * time.Second)
			continue
		}
		if queryErr == utils.DbErrNotFound {
			currentAccountCounts = 0
		} else if queryErr != nil {
			panic(queryErr.Error())
		} else {
			currentAccountCounts = int(latestIdx) + 1
		}
		break
	}

	// Step 3: Sort account asset keys.
	accountAssetKeys := make([]int, 0, len(s.accounts))
	for k, accounts := range s.accounts {
		accountAssetKeys = append(accountAssetKeys, k)
		fmt.Println("the asset counts of user is", k, "total ops number is", len(accounts))
	}
	sort.Ints(accountAssetKeys)

	totalAccountCounts := 0
	for _, accounts := range s.accounts {
		totalAccountCounts += len(accounts)
	}
	fmt.Println("total accounts num (including padding)", totalAccountCounts)

	// Step 4: Compute accountTreeRoot as hex string.
	accountTreeRoot := hex.EncodeToString(s.accountTree.Root())

	// Step 5: Process accounts in parallel per tier, write to DB in order.
	// Segments are computed in parallel and sent as whole slices through segCh.
	// A DB writer goroutine drains segCh in order, so computation of segment N+1
	// overlaps with DB writes of segment N (pipeline parallelism).
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}

	const segmentSize = 10000
	const segmentBuffer = 2 // pipeline depth: bounds memory to ~(1+segmentBuffer)*segmentSize UserProofs

	segCh := make(chan []UserProof, segmentBuffer)
	quit := make(chan int, 1)
	go writeUserProofDB(segCh, s.userProofModel, quit, currentAccountCounts)

	prevAccountCounts := 0
	for _, k := range accountAssetKeys {
		accounts := s.accounts[k]
		if currentAccountCounts >= len(accounts)+prevAccountCounts {
			prevAccountCounts += len(accounts)
			continue
		}
		startIdx := currentAccountCounts - prevAccountCounts
		count := len(accounts) - startIdx

		// Process in fixed-size segments: parallel compute within each segment,
		// then send to DB writer in AccountIndex order. This bounds memory to
		// ~segmentSize * channelBuffer * sizeof(UserProof) while ensuring
		// ordered writes so that count-based resume is correct after a crash.
		for segStart := 0; segStart < count; segStart += segmentSize {
			segEnd := segStart + segmentSize
			if segEnd > count {
				segEnd = count
			}
			segCount := segEnd - segStart
			proofs := make([]UserProof, segCount)

			chunkSize := (segCount + workers - 1) / workers
			var wg sync.WaitGroup
			for w := 0; w < workers; w++ {
				lo := w * chunkSize
				hi := lo + chunkSize
				if hi > segCount {
					hi = segCount
				}
				if lo >= hi {
					break
				}
				wg.Add(1)
				go func(accs []utils.AccountInfo, baseIdx, lo, hi int) {
					defer wg.Done()
					for i := lo; i < hi; i++ {
						acc := &accs[baseIdx+i]
						leaf := s.accountTree.Get(acc.AccountIndex)
						proof, err := s.accountTree.GetProof(acc.AccountIndex)
						if err != nil {
							panic(err.Error())
						}
						proofs[i] = *convertAccount(acc, leaf, proof, accountTreeRoot)
					}
				}(accounts, startIdx+segStart, lo, hi)
			}
			wg.Wait()

			segCh <- proofs
		}

		prevAccountCounts += len(accounts)
		currentAccountCounts = prevAccountCounts
	}

	close(segCh)
	totalWritten := <-quit

	fmt.Println("total write", totalWritten)
	if currentAccountCounts != totalAccountCounts {
		fmt.Println("totalCounts actual:expected", currentAccountCounts, totalAccountCounts)
		panic("mismatch num")
	}
	fmt.Println("userproof service run finished...")
}

// convertAccount builds a UserProof (including UserConfig JSON) from an account.
func convertAccount(account *utils.AccountInfo, leafHash []byte, proof [][]byte, root string) *UserProof {
	var userProof UserProof
	var userConfig UserConfig

	userProof.AccountIndex = account.AccountIndex
	userProof.AccountId = hex.EncodeToString(account.AccountId)
	userProof.AccountLeafHash = hex.EncodeToString(leafHash)

	proofSerial, err := json.Marshal(proof)
	if err != nil {
		panic(err.Error())
	}
	userProof.Proof = string(proofSerial)

	assets, err := json.Marshal(account.Assets)
	if err != nil {
		panic(err.Error())
	}
	userProof.Assets = string(assets)
	userProof.TotalDebt = account.TotalDebt.String()
	userProof.TotalEquity = account.TotalEquity.String()
	userProof.TotalCollateral = account.TotalCollateral.String()

	userConfig.AccountIndex = account.AccountIndex
	userConfig.AccountIdHash = hex.EncodeToString(account.AccountId)
	userConfig.Proof = proof
	userConfig.Root = root
	userConfig.Assets = account.Assets
	userConfig.TotalDebt = account.TotalDebt
	userConfig.TotalEquity = account.TotalEquity
	userConfig.TotalCollateral = account.TotalCollateral

	configSerial, err := json.Marshal(userConfig)
	if err != nil {
		panic(err.Error())
	}
	userProof.Config = string(configSerial)
	return &userProof
}

// writeUserProofDB receives ordered segments of user proofs via segCh and
// batch-writes them to MySQL (dbBatchSize per DB call). Sends totalWritten
// to quit when done.
func writeUserProofDB(segCh <-chan []UserProof, userProofModel UserProofModel, quit chan<- int, totalWritten int) {
	const dbBatchSize = 100
	for proofs := range segCh {
		for i := 0; i < len(proofs); i += dbBatchSize {
			end := i + dbBatchSize
			if end > len(proofs) {
				end = len(proofs)
			}
			if err := userProofModel.CreateUserProofs(proofs[i:end]); err != nil {
				panic(err.Error())
			}
			totalWritten += end - i
			if totalWritten%100000 == 0 {
				fmt.Println("write", totalWritten, "proof to db")
			}
		}
	}
	quit <- totalWritten
}
