package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/binance/zkmerkle-proof-of-solvency/src/userproof/config"
	"github.com/binance/zkmerkle-proof-of-solvency/src/userproof/model"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	bsmt "github.com/bnb-chain/zkbnb-smt"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io/ioutil"
	"log"
	"os"
	"time"
)

func HandleUserData(userProofConfig *config.Config) []utils.AccountInfo {
	startTime := time.Now().UnixMilli()
	accounts, _, err := utils.ParseUserDataSet(userProofConfig.UserDataFile)
	if err != nil {
		panic(err.Error())
	}

	endTime := time.Now().UnixMilli()
	fmt.Println("handle user data cost ", endTime-startTime, " ms")
	return accounts
}

type AccountLeave struct {
	hash  []byte
	index uint32
}

func ComputeAccountRootHash(userProofConfig *config.Config) {
	accountTree, err := utils.NewAccountTree("memory", "")
	if err != nil {
		panic(err.Error())
	}
	accounts, _, err := utils.ParseUserDataSet(userProofConfig.UserDataFile)
	if err != nil {
		panic(err.Error())
	}
	startTime := time.Now().UnixMilli()
	totalOpsNumber := len(accounts)
	fmt.Println("total ops number is ", totalOpsNumber)
	chs := make(chan AccountLeave, 1000)
	workers := 32
	results := make(chan bool, workers)
	averageAccounts := (totalOpsNumber + workers - 1) / workers
	actualWorkers := 0
	for i := 0; i < workers; i++ {
		srcAccountIndex := i * averageAccounts
		destAccountIndex := (i + 1) * averageAccounts
		if destAccountIndex > totalOpsNumber {
			destAccountIndex = totalOpsNumber
		}
		go CalculateAccountHash(accounts[srcAccountIndex:destAccountIndex], chs, results)
		if destAccountIndex == totalOpsNumber {
			actualWorkers = i + 1
			break
		}
	}
	fmt.Println("actual workers is ", actualWorkers)
	quit := make(chan bool, 1)
	go CalculateAccountTreeRoot(chs, &accountTree, quit)

	for i := 0; i < actualWorkers; i++ {
		<-results
	}
	close(chs)
	<-quit
	endTime := time.Now().UnixMilli()
	fmt.Println("user account tree generation cost ", endTime-startTime, " ms")
	fmt.Printf("account tree root %x\n", accountTree.Root())
}

func CalculateAccountHash(accounts []utils.AccountInfo, chs chan<- AccountLeave, res chan<- bool) {
	poseidonHasher := poseidon.NewPoseidon()
	for i := 0; i < len(accounts); i++ {
		chs <- AccountLeave{
			hash:  utils.AccountInfoToHash(&accounts[i], &poseidonHasher),
			index: accounts[i].AccountIndex,
		}
	}
	res <- true
}

func CalculateAccountTreeRoot(accountLeaves <-chan AccountLeave, accountTree *bsmt.SparseMerkleTree, quit chan<- bool) {
	num := 0
	for accountLeaf := range accountLeaves {
		(*accountTree).Set(uint64(accountLeaf.index), accountLeaf.hash)
		num++
		if num%100000 == 0 {
			fmt.Println("for now, already set ", num, " accounts in tree")
		}
	}
	quit <- true
}

func main() {
	memoryTreeFlag := flag.Bool("memory_tree", false, "construct memory merkle tree")
	remotePasswdConfig := flag.String("remote_password_config", "", "fetch password from aws secretsmanager")
	flag.Parse()
	userProofConfig := &config.Config{}
	content, err := ioutil.ReadFile("config/config.json")
	if err != nil {
		panic(err.Error())
	}
	err = json.Unmarshal(content, userProofConfig)
	if err != nil {
		panic(err.Error())
	}
	if *remotePasswdConfig != "" {
		s, err := utils.GetPostgresqlSource(userProofConfig.PostgresDataSource, *remotePasswdConfig)
		if err != nil {
			panic(err.Error())
		}
		userProofConfig.PostgresDataSource = s
	}
	if *memoryTreeFlag {
		ComputeAccountRootHash(userProofConfig)
		return
	}
	// ComputeAccountRootHash(userProofConfig)
	accountTree, err := utils.NewAccountTree(userProofConfig.TreeDB.Driver, userProofConfig.TreeDB.Option.Addr)
	accounts := HandleUserData(userProofConfig)
	fmt.Println("num", len(accounts))
	userProofModel := OpenUserProofTable(userProofConfig)
	latestAccountIndex, err := userProofModel.GetLatestAccountIndex()
	if err != nil && err != utils.DbErrNotFound {
		panic(err.Error())
	}
	if err == nil {
		latestAccountIndex += 1
	}
	accountTreeRoot := hex.EncodeToString(accountTree.Root())
	// proofs := make([]model.UserProof, 1)
	jobs := make(chan Job, 1000)
	nums := make(chan int, 1)
	results := make(chan *model.UserProof, 1000)
	for i := 0; i < 1; i++ {
		go worker(jobs, results, nums, accountTreeRoot)
	}
	quit := make(chan int, 1)
	for i := 0; i < 1; i++ {
		go WriteDB(results, userProofModel, quit, latestAccountIndex)
	}
	for i := int(latestAccountIndex); i < len(accounts); i++ {
		leaf, err := accountTree.Get(uint64(i), nil)
		if err != nil {
			panic(err.Error())
		}
		proof, err := accountTree.GetProof(uint64(accounts[i].AccountIndex))
		if err != nil {
			panic(err.Error())
		}
		jobs <- Job{
			account: &accounts[i],
			proof:   proof,
			leaf:    leaf,
		}
	}
	close(jobs)
	totalCounts := int(latestAccountIndex)
	for i := 0; i < 1; i++ {
		num := <-nums
		totalCounts += num
		fmt.Println("totalCounts", totalCounts)
	}
	if totalCounts != len(accounts) {
		fmt.Println("totalCounts actual:expected", totalCounts, len(accounts))
		panic("mismatch num")
	}
	close(results)
	for i := 0; i < 1; i++ {
		<-quit
	}
	fmt.Println("userproof service run finished...")
}

func WriteDB(results <-chan *model.UserProof, userProofModel model.UserProofModel, quit chan<- int, latestAccountIndex uint32) {
	index := 0
	proofs := make([]model.UserProof, 100)
	num := int(latestAccountIndex)
	for proof := range results {
		proofs[index] = *proof
		index += 1
		if index%100 == 0 {
			error := userProofModel.CreateUserProofs(proofs)
			if error != nil {
				panic(error.Error())
			}
			num += 100
			if num%100000 == 0 {
				fmt.Println("write ", num, "proof to db")
			}
			index = 0
		}
	}
	proofs = proofs[:index]
	if index > 0 {
		fmt.Println("write ", len(proofs), "proofs to db")
		userProofModel.CreateUserProofs(proofs)
		num += index
	}
	fmt.Println("total write ", num)
	quit <- 0
}

type Job struct {
	account *utils.AccountInfo
	proof   [][]byte
	leaf    []byte
}

func worker(jobs <-chan Job, results chan<- *model.UserProof, nums chan<- int, root string) {
	num := 0
	for job := range jobs {
		userProof := ConvertAccount(job.account, job.leaf, job.proof, root)
		results <- userProof
		num += 1
	}
	nums <- num
}

func ConvertAccount(account *utils.AccountInfo, leafHash []byte, proof [][]byte, root string) *model.UserProof {
	var userProof model.UserProof
	var userConfig model.UserConfig
	userProof.AccountIndex = account.AccountIndex
	userProof.AccountId = hex.EncodeToString(account.AccountId)
	userProof.AccountLeafHash = hex.EncodeToString(leafHash)
	proofSerial, err := json.Marshal(proof)
	userProof.Proof = string(proofSerial)
	assets, err := json.Marshal(account.Assets)
	if err != nil {
		panic(err.Error())
	}
	userProof.Assets = string(assets)
	userProof.TotalDebt = account.TotalDebt.String()
	userProof.TotalEquity = account.TotalEquity.String()

	userConfig.AccountIndex = account.AccountIndex
	userConfig.AccountIdHash = hex.EncodeToString(account.AccountId)
	userConfig.Proof = proof
	userConfig.Root = root
	userConfig.Assets = account.Assets
	userConfig.TotalDebt = account.TotalDebt
	userConfig.TotalEquity = account.TotalEquity
	configSerial, err := json.Marshal(userConfig)
	if err != nil {
		panic(err.Error())
	}
	userProof.Config = string(configSerial)
	return &userProof
}

func OpenUserProofTable(userConfig *config.Config) model.UserProofModel {
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             60 * time.Second, // Slow SQL threshold
			LogLevel:                  logger.Silent,    // Log level
			IgnoreRecordNotFoundError: true,             // Ignore ErrRecordNotFound error for logger
			Colorful:                  false,            // Disable color
		},
	)
	db, err := gorm.Open(postgres.Open(userConfig.PostgresDataSource), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		panic(err.Error())
	}
	userProofTable := model.NewUserProofModel(db, userConfig.DbSuffix)
	userProofTable.CreateUserProofTable()
	return userProofTable
}
