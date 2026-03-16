package witness

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"log"
	"math/big"
	"os"
	"runtime"
	"sort"
	"sync/atomic"
	"time"

	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils/merkletree"
	"github.com/binance/zkmerkle-proof-of-solvency/src/witness/config"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
	"github.com/klauspost/compress/s2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Witness struct {
	accountTree              *merkletree.FixedDepthMerkleTree
	witnessModel             WitnessModel
	ops                      map[int][]utils.AccountInfo
	cexAssets                []utils.CexAssetInfo
	db                       *gorm.DB
	ch                       chan BatchWitness
	quit                     chan int
	currentBatchNumber       int64
	batchNumberMappingKeys   []int
	batchNumberMappingValues []int
}

func NewWitness(accountTree *merkletree.FixedDepthMerkleTree,
	ops map[int][]utils.AccountInfo, cexAssets []utils.CexAssetInfo,
	config *config.Config) *Witness {
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             60 * time.Second,
			LogLevel:                  logger.Silent,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
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
		witnessModel:       NewWitnessModel(db, config.DbSuffix),
		ops:                ops,
		cexAssets:          cexAssets,
		db:                 db,
		ch:                 make(chan BatchWitness, 100),
		quit:               make(chan int, 1),
		currentBatchNumber: 0,
	}
}

// GetDB returns the underlying database connection so it can be shared
// with other services (e.g., UserProofService).
func (w *Witness) GetDB() *gorm.DB {
	return w.db
}

// serializeJob holds the data needed to serialize and compress a batch witness.
type serializeJob struct {
	height int64
	wit    *utils.BatchCreateUserWitness
	done   chan BatchWitness // single-element channel to deliver the result
}

func (w *Witness) Run() {
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
	fmt.Println("starting witness generation...")

	// Start pipeline stages.
	go w.WriteBatchWitnessToDB()

	serializeWorkers := max(runtime.NumCPU()/2, 2)
	jobCh := make(chan serializeJob, serializeWorkers)
	// ordered queue: main loop appends done channels, collector reads them in order.
	orderCh := make(chan chan BatchWitness, serializeWorkers)

	// Start serialize+compress worker pool.
	for i := 0; i < serializeWorkers; i++ {
		go serializeWorker(jobCh)
	}

	// Collector: reads results in submission order and forwards to DB writer.
	go func() {
		for done := range orderCh {
			w.ch <- <-done
		}
		close(w.ch)
	}()

	// Main loop: generate witness data (serial), dispatch serialization (parallel).
	accountTreeRoot := w.accountTree.Root()
	poseidonHasher := poseidon.NewPoseidon()

	userOpsPerBatch := 0
	startBatchNum := 0
	recoveredBatchNum := int(height)
	for p, k := range w.batchNumberMappingKeys {
		endBatchNum := w.batchNumberMappingValues[p]
		userOpsPerBatch = utils.BatchCreateUserOpsCountsTiers[k]

		for i := startBatchNum; i < endBatchNum; i++ {
			if i <= recoveredBatchNum {
				continue
			}
			batchCreateUserWit := &utils.BatchCreateUserWitness{
				AccountTreeRoot: accountTreeRoot,
				BeforeCexAssets: make([]utils.CexAssetInfo, utils.AssetCounts),
				CreateUserOps:   make([]utils.CreateUserOperation, userOpsPerBatch),
			}

			copy(batchCreateUserWit.BeforeCexAssets[:], w.cexAssets[:])
			for j := 0; j < len(w.cexAssets); j++ {
				commitments := utils.ConvertAssetInfoToBytes(w.cexAssets[j])
				for c := 0; c < len(commitments); c++ {
					poseidonHasher.Write(commitments[c])
				}
			}
			batchCreateUserWit.BeforeCEXAssetsCommitment = poseidonHasher.Sum(nil)
			poseidonHasher.Reset()

			relativeBatchNum := i - startBatchNum
			for j := relativeBatchNum * userOpsPerBatch; j < (relativeBatchNum+1)*userOpsPerBatch; j++ {
				w.fillCreateUserOp(k, uint32(j), uint32(relativeBatchNum*userOpsPerBatch), batchCreateUserWit)
			}

			batchCreateUserWit.MinAccountIndex = batchCreateUserWit.CreateUserOps[0].AccountIndex
			batchCreateUserWit.MaxAccountIndex = batchCreateUserWit.CreateUserOps[userOpsPerBatch-1].AccountIndex

			for j := 0; j < len(w.cexAssets); j++ {
				commitments := utils.ConvertAssetInfoToBytes(w.cexAssets[j])
				for c := 0; c < len(commitments); c++ {
					poseidonHasher.Write(commitments[c])
				}
			}
			batchCreateUserWit.AfterCEXAssetsCommitment = poseidonHasher.Sum(nil)
			poseidonHasher.Reset()

			minBytes := new(big.Int).SetUint64(uint64(batchCreateUserWit.MinAccountIndex)).Bytes()
			if len(minBytes) == 0 {
				minBytes = []byte{0}
			}
			maxBytes := new(big.Int).SetUint64(uint64(batchCreateUserWit.MaxAccountIndex)).Bytes()
			if len(maxBytes) == 0 {
				maxBytes = []byte{0}
			}
			batchCreateUserWit.BatchCommitment = poseidon.PoseidonBytes(
				batchCreateUserWit.AccountTreeRoot,
				batchCreateUserWit.BeforeCEXAssetsCommitment,
				batchCreateUserWit.AfterCEXAssetsCommitment,
				minBytes,
				maxBytes)

			// Dispatch to serialize worker pool.
			done := make(chan BatchWitness, 1)
			orderCh <- done
			jobCh <- serializeJob{height: int64(i), wit: batchCreateUserWit, done: done}
		}
		startBatchNum = endBatchNum
	}

	close(jobCh)   // signal workers to exit
	close(orderCh) // signal collector to exit after draining
	<-w.quit
	fmt.Printf("witness run finished, the account tree root is %x\n", w.accountTree.Root())
}

// serializeWorker encodes and compresses batch witnesses.
func serializeWorker(jobs <-chan serializeJob) {
	var serializeBuf bytes.Buffer
	var compressedBuf []byte
	for job := range jobs {
		serializeBuf.Reset()
		enc := gob.NewEncoder(&serializeBuf)
		err := enc.Encode(job.wit)
		if err != nil {
			panic(err.Error())
		}
		compressedBuf = s2.Encode(compressedBuf[:0], serializeBuf.Bytes())
		job.done <- BatchWitness{
			Height:      job.height,
			WitnessData: base64.StdEncoding.EncodeToString(compressedBuf),
			Status:      StatusPublished,
		}
	}
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
	const (
		// Measured in local test: one witness row is ~312 KiB on average.
		// Keep a practical default batch size for throughput.
		defaultDBBatchSize = 25
		// Safety cap for one INSERT payload. If exceeded, degrade to single-row writes.
		maxInsertPayloadBytes = 16 * 1024 * 1024
	)
	dbBatchSize := defaultDBBatchSize

	estimateBatchSize := func(batch []BatchWitness) int {
		totalSize := 0
		for _, row := range batch {
			totalSize += len(row.WitnessData)
		}
		return totalSize
	}

	persist := func(rows []BatchWitness) {
		err := w.witnessModel.CreateBatchWitness(rows)
		if err != nil {
			panic("create batch witness failed " + err.Error())
		}
		last := rows[len(rows)-1]
		atomic.StoreInt64(&w.currentBatchNumber, last.Height)
		if last.Height%100 == 0 {
			fmt.Println("save batch ", last.Height, " to db")
		}
	}

	flush := func(batch []BatchWitness) {
		if len(batch) == 0 {
			return
		}
		totalSize := estimateBatchSize(batch)
		if totalSize > maxInsertPayloadBytes && dbBatchSize != 1 {
			dbBatchSize = 1
			fmt.Printf("witness DB payload %d bytes exceeds 16MB, fallback dbBatchSize=1\n", totalSize)
		}

		if dbBatchSize == 1 && len(batch) > 1 {
			for i := range batch {
				row := batch[i : i+1]
				if len(row[0].WitnessData) > maxInsertPayloadBytes {
					fmt.Printf("warning: single witness payload still exceeds 16MB at height %d\n", row[0].Height)
				}
				persist(row)
			}
			return
		}

		if totalSize > maxInsertPayloadBytes {
			fmt.Printf("warning: single INSERT payload still exceeds 16MB (%d bytes)\n", totalSize)
		}
		persist(batch)
	}

	batch := make([]BatchWitness, 0, defaultDBBatchSize)
	for witness := range w.ch {
		batch = append(batch, witness)
		if len(batch) < dbBatchSize {
			continue
		}
		flush(batch)
		batch = batch[:0]
	}

	// Flush remaining rows when input channel is closed.
	flush(batch)
	w.quit <- 0
}

// fillCreateUserOp populates one CreateUserOperation from the final built tree.
func (w *Witness) fillCreateUserOp(assetKey int, accountIndex uint32, currentAccountIndex uint32, batchCreateUserWit *utils.BatchCreateUserWitness) {
	index := accountIndex - currentAccountIndex
	account := w.ops[assetKey][accountIndex]

	accountProof, err := w.accountTree.GetProof(account.AccountIndex)
	if err != nil {
		panic(err.Error())
	}
	copy(batchCreateUserWit.CreateUserOps[index].AccountProof[:], accountProof[:])

	for p := 0; p < len(account.Assets); p++ {
		w.cexAssets[account.Assets[p].Index].TotalEquity = utils.SafeAdd(w.cexAssets[account.Assets[p].Index].TotalEquity, account.Assets[p].Equity)
		w.cexAssets[account.Assets[p].Index].TotalDebt = utils.SafeAdd(w.cexAssets[account.Assets[p].Index].TotalDebt, account.Assets[p].Debt)
		w.cexAssets[account.Assets[p].Index].LoanCollateral = utils.SafeAdd(w.cexAssets[account.Assets[p].Index].LoanCollateral, account.Assets[p].Loan)
		w.cexAssets[account.Assets[p].Index].MarginCollateral = utils.SafeAdd(w.cexAssets[account.Assets[p].Index].MarginCollateral, account.Assets[p].Margin)
		w.cexAssets[account.Assets[p].Index].PortfolioMarginCollateral = utils.SafeAdd(w.cexAssets[account.Assets[p].Index].PortfolioMarginCollateral, account.Assets[p].PortfolioMargin)
	}

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
