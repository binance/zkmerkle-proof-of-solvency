package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/binance/zkmerkle-proof-of-solvency/src/dbtool/config"
	"github.com/binance/zkmerkle-proof-of-solvency/src/prover/prover"
	"github.com/binance/zkmerkle-proof-of-solvency/src/userproof/model"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/binance/zkmerkle-proof-of-solvency/src/witness/witness"
	"github.com/go-redis/redis/v8"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io/ioutil"
	"log"
	"os"
	"time"
)

func main() {
	dbtoolConfig := &config.Config{}
	content, err := ioutil.ReadFile("config/config.json")
	if err != nil {
		panic(err.Error())
	}
	err = json.Unmarshal(content, dbtoolConfig)
	if err != nil {
		panic(err.Error())
	}

	onlyFlushKvrocks := flag.Bool("only_delete_kvrocks", false, "only delete kvrocks")
	deleteAllData := flag.Bool("delete_all", false, "delete kvrocks and postgresql data")
	checkProverStatus := flag.Bool("check_prover_status", false, "check prover status")
	remotePasswdConfig := flag.String("remote_password_config", "", "fetch password from aws secretsmanager")
	queryCexAssetsConfig := flag.Bool("query_cex_assets", false, "query cex assets info")

	flag.Parse()

	if *remotePasswdConfig != "" {
		s, err := utils.GetPostgresqlSource(dbtoolConfig.PostgresDataSource, *remotePasswdConfig)
		if err != nil {
			panic(err.Error())
		}
		dbtoolConfig.PostgresDataSource = s
	}
	if *deleteAllData {
		db, err := gorm.Open(postgres.Open(dbtoolConfig.PostgresDataSource))
		if err != nil {
			panic(err.Error())
		}
		witnessModel := witness.NewWitnessModel(db, dbtoolConfig.DbSuffix)
		err = witnessModel.DropBatchWitnessTable()
		if err != nil {
			fmt.Println("drop witness table failed")
			panic(err.Error())
		}
		fmt.Println("drop witness table successfully")

		proofModel := prover.NewProofModel(db, dbtoolConfig.DbSuffix)
		err = proofModel.DropProofTable()
		if err != nil {
			fmt.Println("drop proof table failed")
			panic(err.Error())
		}
		fmt.Println("drop proof table successfully")

		userProofModel := model.NewUserProofModel(db, dbtoolConfig.DbSuffix)
		err = userProofModel.DropUserProofTable()
		if err != nil {
			fmt.Println("drop userproof table failed")
			panic(err.Error())
		}
		fmt.Println("drop userproof table successfully")
	}

	if *deleteAllData || *onlyFlushKvrocks {
		client := redis.NewClient(&redis.Options{
			Addr:            dbtoolConfig.TreeDB.Option.Addr,
			PoolSize:        500,
			MaxRetries:      5,
			MinRetryBackoff: 8 * time.Millisecond,
			MaxRetryBackoff: 512 * time.Millisecond,
			DialTimeout:     10 * time.Second,
			ReadTimeout:     10 * time.Second,
			WriteTimeout:    10 * time.Second,
			PoolTimeout:     15 * time.Second,
			IdleTimeout:     5 * time.Minute,
		})
		client.FlushAll(context.Background())
		fmt.Println("kvrocks data drop successfully")
	}

	if *checkProverStatus {
		newLogger := logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
			logger.Config{
				SlowThreshold:             60 * time.Second, // Slow SQL threshold
				LogLevel:                  logger.Silent,    // Log level
				IgnoreRecordNotFoundError: true,             // Ignore ErrRecordNotFound error for logger
				Colorful:                  false,            // Disable color
			},
		)
		db, err := gorm.Open(postgres.Open(dbtoolConfig.PostgresDataSource), &gorm.Config{
			Logger: newLogger,
		})
		if err != nil {
			panic(err.Error())
		}
		witnessModel := witness.NewWitnessModel(db, dbtoolConfig.DbSuffix)
		proofModel := prover.NewProofModel(db, dbtoolConfig.DbSuffix)

		witnessCounts, err := witnessModel.GetRowCounts()
		if err != nil {
			panic(err.Error())
		}
		proofCounts, err := proofModel.GetRowCounts()
		fmt.Printf("Total witness item %d, Published item %d, Pending item %d, Finished item %d\n", witnessCounts[0], witnessCounts[1], witnessCounts[2], witnessCounts[3])
		fmt.Println(witnessCounts[0] - proofCounts)
	}

	if *queryCexAssetsConfig {
		db, err := gorm.Open(postgres.Open(dbtoolConfig.PostgresDataSource))
		if err != nil {
			panic(err.Error())
		}
		witnessModel := witness.NewWitnessModel(db, dbtoolConfig.DbSuffix)
		latestWitness, err := witnessModel.GetLatestBatchWitness()
		if err != nil {
			panic(err.Error())
		}
		witness := utils.DecodeBatchWitness(latestWitness.WitnessData)
		if witness == nil {
			panic("decode invalid witness data")
		}
		cexAssetsInfo := utils.RecoverAfterCexAssets(witness)
		var newAssetsInfo []utils.CexAssetInfo
		for i := 0; i < len(cexAssetsInfo); i++ {
			if cexAssetsInfo[i].BasePrice != 0 {
				newAssetsInfo = append(newAssetsInfo, cexAssetsInfo[i])
			}
		}
		cexAssetsInfoBytes, _ := json.Marshal(newAssetsInfo)
		fmt.Println(string(cexAssetsInfoBytes))
	}
}
