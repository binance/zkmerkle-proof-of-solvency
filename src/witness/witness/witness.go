package witness

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"runtime"
	"sort"
	"sync/atomic"
	"time"

	"sync"

	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/binance/zkmerkle-proof-of-solvency/src/witness/config"
	bsmt "github.com/bnb-chain/zkbnb-smt"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"github.com/klauspost/compress/s2"
)

type Witness struct {
	accountTree              bsmt.SparseMerkleTree
	totalOpsNumber           uint32
	witnessModel             WitnessModel
	ops                      map[int][]utils.AccountInfo
	cexAssets                []utils.CexAssetInfo
	db                       *gorm.DB
	ch                       chan BatchWitness
	quit                     chan int
	accountHashChan          map[int][]chan []byte
	currentBatchNumber       int64
	batchNumberMappingKeys   []int
	batchNumberMappingValues []int
}

func NewWitness(accountTree bsmt.SparseMerkleTree, totalOpsNumber uint32,
	ops map[int][]utils.AccountInfo, cexAssets []utils.CexAssetInfo,
	config *config.Config) *Witness {
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             60 * time.Second, // Slow SQL threshold
			LogLevel:                  logger.Silent,    // Log level
			IgnoreRecordNotFoundError: true,             // Ignore ErrRecordNotFound error for logger
			Colorful:                  false,            // Disable color
		},
	)
	db, err := gorm.Open(mysql.Open(config.MysqlDataSource), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		panic(err.Error())
	}

	return &Witness{
		accountTree:        accountTree,
		totalOpsNumber:     totalOpsNumber,
		witnessModel:       NewWitnessModel(db, config.DbSuffix),
		ops:                ops,
		cexAssets:          cexAssets,
		ch:                 make(chan BatchWitness, 100),
		quit:               make(chan int, 1),
		currentBatchNumber: 0,
		accountHashChan:    make(map[int][]chan []byte),
	}
}

func (w *Witness) Run() {
	// create table first
	w.witnessModel.CreateBatchWitnessTable()
	var latestWitness *BatchWitness
	var err error
	for {
		latestWitness, err = w.witnessModel.GetLatestBatchWitness()
		if err == utils.DbErrQueryInterrupted || err == utils.DbErrQueryTimeout {
			fmt.Println("get latest witness timeout, retry...:", err.Error())
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}
	var height int64
	if err == utils.DbErrNotFound {
		height = -1
	}
	if err != nil && err != utils.DbErrNotFound {
		panic(err.Error())
	}
	if err == nil {
		height = latestWitness.Height
		w.cexAssets = w.GetCexAssets(latestWitness)
	}
	batchNumber := w.GetBatchNumber()
	if height == int64(batchNumber)-1 {
		fmt.Println("already generate all accounts witness")
		return
	}
	w.currentBatchNumber = height
	fmt.Println("latest height is ", height)

	// tree version
	if w.accountTree.LatestVersion() > bsmt.Version(height+1) {
		rollbackVersion := bsmt.Version(height + 1)
		err = w.accountTree.Rollback(rollbackVersion)
		if err != nil {
			fmt.Println("rollback failed ", rollbackVersion, err.Error())
			panic("rollback failed")
		} else {
			fmt.Printf("rollback to %x\n", w.accountTree.Root())
		}
	} else if w.accountTree.LatestVersion() < bsmt.Version(height+1) {
		panic("account tree version is less than current height")
	} else {
		fmt.Println("normal starting...")
	}

	w.PaddingAccounts()

	poseidonHasher := poseidon.NewPoseidon()
	go w.WriteBatchWitnessToDB()
	for k := range w.ops {
		w.accountHashChan[k] = make([]chan []byte, utils.BatchCreateUserOpsCountsTiers[k])
		for p := 0; p < utils.BatchCreateUserOpsCountsTiers[k]; p++ {
			w.accountHashChan[k][p] = make(chan []byte, 1)
		}
	}

	cpuCores := runtime.NumCPU()
	workersNum := 1
	if cpuCores > 2 {
		workersNum = cpuCores - 2
	}

	userOpsPerBatch := 0
	startBatchNum := 0
	recoveredBatchNum := int(height)
	for p, k := range w.batchNumberMappingKeys {
		var wg sync.WaitGroup
		endBatchNum := w.batchNumberMappingValues[p]
		userOpsPerBatch = utils.BatchCreateUserOpsCountsTiers[k]
		averageCount := userOpsPerBatch/workersNum + 1
		for i := 0; i < workersNum; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				for j := startBatchNum; j < endBatchNum; j++ {
					if j <= recoveredBatchNum {
						continue
					}
					if index*averageCount >= userOpsPerBatch {
						break
					}
					lowAccountIndex := index*averageCount + (j-startBatchNum)*userOpsPerBatch
					highAccountIndex := averageCount + lowAccountIndex
					if highAccountIndex > (j-startBatchNum+1)*userOpsPerBatch {
						highAccountIndex = (j - startBatchNum + 1) * userOpsPerBatch
					}
					currentAccountIndex := (j - startBatchNum) * userOpsPerBatch
					// fmt.Printf("worker num: %d, lowAccountInde: %d, highAccountIndex: %d, current: %d\n", index, lowAccountIndex, highAccountIndex, currentAccountIndex)
					w.ComputeAccountHash(k, uint32(lowAccountIndex), uint32(highAccountIndex), uint32(currentAccountIndex))
				}
			}(i)
		}
		for i := startBatchNum; i < endBatchNum; i++ {
			if i <= recoveredBatchNum {
				continue
			}
			batchCreateUserWit := &utils.BatchCreateUserWitness{
				BeforeAccountTreeRoot: w.accountTree.Root(),
				BeforeCexAssets:       make([]utils.CexAssetInfo, utils.AssetCounts),
				CreateUserOps:         make([]utils.CreateUserOperation, userOpsPerBatch),
			}

			copy(batchCreateUserWit.BeforeCexAssets[:], w.cexAssets[:])
			for j := 0; j < len(w.cexAssets); j++ {
				commitments := utils.ConvertAssetInfoToBytes(w.cexAssets[j])
				for p := 0; p < len(commitments); p++ {
					poseidonHasher.Write(commitments[p])
				}
			}
			batchCreateUserWit.BeforeCEXAssetsCommitment = poseidonHasher.Sum(nil)
			poseidonHasher.Reset()

			relativeBatchNum := i - startBatchNum
			for j := relativeBatchNum * userOpsPerBatch; j < (relativeBatchNum+1)*userOpsPerBatch; j++ {
				w.ExecuteBatchCreateUser(k, uint32(j), uint32(relativeBatchNum*userOpsPerBatch), batchCreateUserWit)
			}
			for j := 0; j < len(w.cexAssets); j++ {
				commitments := utils.ConvertAssetInfoToBytes(w.cexAssets[j])
				for p := 0; p < len(commitments); p++ {
					poseidonHasher.Write(commitments[p])
				}
			}
			batchCreateUserWit.AfterCEXAssetsCommitment = poseidonHasher.Sum(nil)
			poseidonHasher.Reset()
			batchCreateUserWit.AfterAccountTreeRoot = w.accountTree.Root()

			// compute batch commitment
			batchCreateUserWit.BatchCommitment = poseidon.PoseidonBytes(batchCreateUserWit.BeforeAccountTreeRoot,
				batchCreateUserWit.AfterAccountTreeRoot,
				batchCreateUserWit.BeforeCEXAssetsCommitment,
				batchCreateUserWit.AfterCEXAssetsCommitment)
			// bz, err := json.Marshal(batchCreateUserWit)
			var serializeBuf bytes.Buffer
			enc := gob.NewEncoder(&serializeBuf)
			err := enc.Encode(batchCreateUserWit)
			if err != nil {
				panic(err.Error())
			}
			// startTime := time.Now()
			buf := serializeBuf.Bytes()
			compressedBuf := s2.Encode(nil, buf)
			// endTime := time.Now()
			// fmt.Println("compress time is ", endTime.Sub(startTime), " len of compressed buf is ", len(buf), len(compressedBuf))
			witness := BatchWitness{
				Height:      int64(i),
				WitnessData: base64.StdEncoding.EncodeToString(compressedBuf),
				Status:      StatusPublished,
			}
			accPrunedVersion := bsmt.Version(atomic.LoadInt64(&w.currentBatchNumber) + 1)
			ver, err := w.accountTree.Commit(&accPrunedVersion)
			if err != nil {
				fmt.Println("ver is ", ver)
				panic(err.Error())
			}
			// fmt.Printf("ver is %d account tree root is %x\n", ver, w.accountTree.Root())
			w.ch <- witness
		}
		wg.Wait()
		startBatchNum = endBatchNum
	}

	close(w.ch)
	<-w.quit
	// fmt.Println("cex assets info is ", w.cexAssets)
	fmt.Printf("witness run finished, the account tree root is %x\n", w.accountTree.Root())
}

func (w *Witness) GetCexAssets(wit *BatchWitness) []utils.CexAssetInfo {
	witness := utils.DecodeBatchWitness(wit.WitnessData)
	if witness == nil {
		panic("decode invalid witness data")
	}
	cexAssetsInfo := utils.RecoverAfterCexAssets(witness)
	fmt.Println("recover cex assets successfully")
	return cexAssetsInfo
}

func (w *Witness) WriteBatchWitnessToDB() {
	datas := make([]BatchWitness, 1)
	for witness := range w.ch {
		datas[0] = witness
		err := w.witnessModel.CreateBatchWitness(datas)
		if err != nil {
			panic("create batch witness failed " + err.Error())
		}
		atomic.StoreInt64(&w.currentBatchNumber, witness.Height)
		if witness.Height%100 == 0 {
			fmt.Println("save batch ", witness.Height, " to db")
		}
	}
	w.quit <- 0
}

func (w *Witness) ComputeAccountHash(key int, accountIndex uint32, highAccountIndex uint32, currentIndex uint32) {
	poseidonHasher := poseidon.NewPoseidon()
	for i := accountIndex; i < highAccountIndex; i++ {
		w.accountHashChan[key][i-currentIndex] <- utils.AccountInfoToHash(&w.ops[key][i], &poseidonHasher)
	}
}

func (w *Witness) ExecuteBatchCreateUser(assetKey int, accountIndex uint32, currentAccountIndex uint32, batchCreateUserWit *utils.BatchCreateUserWitness) {
	index := accountIndex - currentAccountIndex
	account := w.ops[assetKey][accountIndex]
	batchCreateUserWit.CreateUserOps[index].BeforeAccountTreeRoot = w.accountTree.Root()
	accountProof, err := w.accountTree.GetProof(uint64(account.AccountIndex))
	if err != nil {
		panic(err.Error())
	}
	copy(batchCreateUserWit.CreateUserOps[index].AccountProof[:], accountProof[:])
	for p := 0; p < len(account.Assets); p++ {
		// update cexAssetInfo
		w.cexAssets[account.Assets[p].Index].TotalEquity = utils.SafeAdd(w.cexAssets[account.Assets[p].Index].TotalEquity, account.Assets[p].Equity)
		w.cexAssets[account.Assets[p].Index].TotalDebt = utils.SafeAdd(w.cexAssets[account.Assets[p].Index].TotalDebt, account.Assets[p].Debt)
		w.cexAssets[account.Assets[p].Index].LoanCollateral = utils.SafeAdd(w.cexAssets[account.Assets[p].Index].LoanCollateral, account.Assets[p].Loan)
		w.cexAssets[account.Assets[p].Index].MarginCollateral = utils.SafeAdd(w.cexAssets[account.Assets[p].Index].MarginCollateral, account.Assets[p].Margin)
		w.cexAssets[account.Assets[p].Index].PortfolioMarginCollateral = utils.SafeAdd(w.cexAssets[account.Assets[p].Index].PortfolioMarginCollateral, account.Assets[p].PortfolioMargin)
	}
	// update account tree
	accountHash := <-w.accountHashChan[assetKey][index]
	err = w.accountTree.Set(uint64(account.AccountIndex), accountHash)
	// fmt.Printf("account index %d, hash: %x\n", account.AccountIndex, accountHash)
	if err != nil {
		panic(err.Error())
	}
	batchCreateUserWit.CreateUserOps[index].AfterAccountTreeRoot = w.accountTree.Root()
	batchCreateUserWit.CreateUserOps[index].AccountIndex = account.AccountIndex
	batchCreateUserWit.CreateUserOps[index].AccountIdHash = account.AccountId
	batchCreateUserWit.CreateUserOps[index].Assets = account.Assets
}

func (w *Witness) GetBatchNumber() int {
	b := 0
	keys := make([]int, 0)
	for k := range w.ops {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	w.batchNumberMappingKeys = keys
	w.batchNumberMappingValues = make([]int, len(keys))
	for i, k := range keys {
		opsPerBatch := utils.BatchCreateUserOpsCountsTiers[k]
		b += (len(w.ops[k]) + opsPerBatch - 1) / opsPerBatch
		w.batchNumberMappingValues[i] = b
	}
	return b
}

func (w *Witness) PaddingAccounts() {
	keys := make([]int, 0)
	for k := range w.ops {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	paddingStartIndex := int(w.totalOpsNumber)
	for _, k := range keys {
		paddingStartIndex, w.ops[k] = utils.PaddingAccounts(w.ops[k], k, paddingStartIndex)
	}
}
