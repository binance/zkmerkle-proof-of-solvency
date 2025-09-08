package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/binance/zkmerkle-proof-of-solvency/src/dbtool/config"
	"github.com/binance/zkmerkle-proof-of-solvency/src/prover/prover"
	"github.com/binance/zkmerkle-proof-of-solvency/src/userproof/model"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/binance/zkmerkle-proof-of-solvency/src/witness/witness"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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
	deleteAllData := flag.Bool("delete_all", false, "delete kvrocks and mysql data")
	checkProverStatus := flag.Bool("check_prover_status", false, "check prover status")
	remotePasswdConfig := flag.String("remote_password_config", "", "fetch password from aws secretsmanager")
	queryCexAssetsConfig := flag.Bool("query_cex_assets", false, "query cex assets info")
	queryWitnessData := flag.Int("query_witness_data", -1, "query witness data by height")
	queryAccountData := flag.Int("query_account_data", -1, "query account data by index")
	pushTaskToRedis := flag.Bool("push_task_to_redis", false, "push task to redis")

	flag.Parse()

	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             60 * time.Second, // Slow SQL threshold
			LogLevel:                  logger.Silent,    // Log level
			IgnoreRecordNotFoundError: true,             // Ignore ErrRecordNotFound error for logger
			Colorful:                  false,            // Disable color
		},
	)

	if *remotePasswdConfig != "" {
		s, err := utils.GetMysqlSource(dbtoolConfig.MysqlDataSource, *remotePasswdConfig)
		if err != nil {
			panic(err.Error())
		}
		dbtoolConfig.MysqlDataSource = s
	}
	if *deleteAllData {
		db, err := gorm.Open(mysql.Open(dbtoolConfig.MysqlDataSource))
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

		// clear redis data
		client := redis.NewClient(&redis.Options{
			Addr:            dbtoolConfig.Redis.Host,
			Password:        dbtoolConfig.Redis.Password,
		})
		client.FlushAll(context.Background())
		fmt.Println("redis data drop successfully")
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
		})
		client.FlushAll(context.Background())
		fmt.Println("kvrocks data drop successfully")
	}

	if *checkProverStatus {
		db, err := gorm.Open(mysql.Open(dbtoolConfig.MysqlDataSource), &gorm.Config{
			Logger: newLogger,
		})
		if err != nil {
			panic(err.Error())
		}
		witnessModel := witness.NewWitnessModel(db, dbtoolConfig.DbSuffix)
		proofModel := prover.NewProofModel(db, dbtoolConfig.DbSuffix)

		var witnessCounts []int64
		var proofCounts int64
		for {
			witnessCounts, err = witnessModel.GetRowCounts()
			if err == utils.DbErrQueryInterrupted || err == utils.DbErrQueryTimeout {
				fmt.Println("get witness counts timeout, retry...:", err.Error())
				time.Sleep(1 * time.Second)
				continue
			}
			if err != nil {
				panic(err.Error())
			}
			break
		}
		for {
			proofCounts, err = proofModel.GetRowCounts()
			if err == utils.DbErrQueryInterrupted || err == utils.DbErrQueryTimeout {
				fmt.Println("get proof counts timeout, retry...:", err.Error())
				time.Sleep(1 * time.Second)
				continue
			}
			if err == utils.DbErrTableNotFound {
				fmt.Println("proof table not found")
				proofCounts = 0
				break
			}
			if err != nil {
				panic(err.Error())
			}
			break
		}

		fmt.Printf("Total witness item %d, Published item %d, Pending item %d, Finished item %d\n", witnessCounts[0], witnessCounts[1], witnessCounts[2], witnessCounts[3])
		fmt.Println(witnessCounts[0] - proofCounts)
	}

	if *queryCexAssetsConfig {
		db, err := gorm.Open(mysql.Open(dbtoolConfig.MysqlDataSource))
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

	if *queryWitnessData != -1 {
		db, err := gorm.Open(mysql.Open(dbtoolConfig.MysqlDataSource), &gorm.Config{
			Logger: newLogger,
		})
		if err != nil {
			panic(err.Error())
		}
		witnessModel := witness.NewWitnessModel(db, dbtoolConfig.DbSuffix)

		w, err := witnessModel.GetBatchWitnessByHeight(int64(*queryWitnessData))
		if err != nil {
			panic(err.Error())
		}
		fmt.Printf("%x", w.WitnessData)
	}

	if *queryAccountData != -1 {
		db, err := gorm.Open(mysql.Open(dbtoolConfig.MysqlDataSource), &gorm.Config{
			Logger: newLogger,
		})
		if err != nil {
			panic(err.Error())
		}
		userProofModel := model.NewUserProofModel(db, dbtoolConfig.DbSuffix)

		u, err := userProofModel.GetUserProofByIndex(uint32(*queryAccountData))
		if err != nil {
			panic(err.Error())
		}
		fmt.Println(u.Config)
	}

	if *pushTaskToRedis {
		db, err := gorm.Open(mysql.Open(dbtoolConfig.MysqlDataSource), &gorm.Config{
			Logger: newLogger,
		})
		if err != nil {
			panic(err.Error())
		}
		witnessModel := witness.NewWitnessModel(db, dbtoolConfig.DbSuffix)
		limit := 1024
		offset := 0
		witessStatusList := []int64{witness.StatusPublished}
		taskQueueName := "por_batch_task_queue_" + dbtoolConfig.DbSuffix
		ctx := context.Background()
		redisCli := redis.NewClient(&redis.Options{
			Addr: dbtoolConfig.Redis.Host,
			Password: dbtoolConfig.Redis.Password,
		})
		for _, status := range witessStatusList {
			offset = 0
			for {
				witnessHeights, err := witnessModel.GetAllBatchHeightsByStatus(status, limit, offset)
				if err == utils.DbErrQueryInterrupted || err == utils.DbErrQueryTimeout {
					fmt.Println("get witness heights timeout, retry...:", err.Error())
					time.Sleep(1 * time.Second)
					continue
				}
				if err == utils.DbErrNotFound {
					fmt.Printf("no more witness data with status %d\n", status)
					break
				}

				redisPipe := redisCli.Pipeline()
				for _, height := range witnessHeights {
					redisPipe.LPush(ctx, taskQueueName, height)
				}
				_, err = redisPipe.Exec(ctx)
				if err != nil {
					panic(err.Error())
				} else {
					fmt.Printf("push %d task to redis, offset: %d\n", len(witnessHeights), offset)
				}
				offset += len(witnessHeights)
			}
		}
		fmt.Println("push task to redis successfully")
	}
}
