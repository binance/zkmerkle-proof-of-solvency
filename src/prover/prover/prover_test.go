package prover

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/binance/zkmerkle-proof-of-solvency/src/prover/config"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/binance/zkmerkle-proof-of-solvency/src/witness/witness"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMockProver(t *testing.T) {
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             60 * time.Second, // Slow SQL threshold
			LogLevel:                  logger.Silent,    // Log level
			IgnoreRecordNotFoundError: true,             // Ignore ErrRecordNotFound error for logger
			Colorful:                  false,            // Disable color
		},
	)
	t.Log("TestWitnessModel")
	dbUri := "zkpos:zkpos@123@tcp(127.0.0.1:3306)/zkpos?parseTime=true"
	db, err := gorm.Open(mysql.Open(dbUri), &gorm.Config{Logger: newLogger})
	if err != nil {
		t.Errorf("error: %s\n", err.Error())
	}
	// write test data to db
	witnessTable := witness.NewWitnessModel(db, "test")
	witnessTable.DropBatchWitnessTable()
	fmt.Println("drop witness table successfully")

	err = witnessTable.CreateBatchWitnessTable()
	if err != nil {
		t.Errorf("error: %s\n", err.Error())
	}
	largeArray := bytes.Repeat([]byte{'a'}, 1780)
	
	startTime := time.Now()
	datas := make([]witness.BatchWitness, 100)
	for i := 0; i < 1000; i++ {
		for j := 0; j < 100; j++ {
			status := witness.StatusPublished
			// if j > 90 {
			// 	status = witness.StatusReceived
			// }
			w := witness.BatchWitness{
				Height: int64(100*i + j),
				Status: int64(status),
				WitnessData: string(largeArray),
			}
			datas[j] = w
		}
		err = witnessTable.CreateBatchWitness(datas)
		if err != nil {
			t.Errorf("error: %s\n", err.Error())
		}
	}
	endTime := time.Now()
	fmt.Println("create witness data time is ", endTime.Sub(startTime))

	limit := 1024
	offset := 0
	witessStatusList := []int64{witness.StatusPublished, witness.StatusReceived}
	taskQueueName := "por_batch_task_queue_test"
	ctx := context.Background()
	redisCli := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
	})
	_, err = redisCli.Del(ctx, taskQueueName).Result()
	if err == nil {
		fmt.Println("delete task queue successfully")
	}
	for _, status := range witessStatusList {
		offset = 0
		for {
			witnessHeights, err := witnessTable.GetAllBatchHeightsByStatus(status, limit, offset)
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
	taskLen, err := redisCli.LLen(ctx, taskQueueName).Result()
	if err != nil {
		panic(err.Error())
	}
	if taskLen != 100000 {
		t.Fatal("task queue length is not equal to 100000")
	}

	config := &config.Config{
		MysqlDataSource: dbUri,
		DbSuffix: "test",
		Redis: struct {
			Host     string
			Password string
		} {
			Host: "127.0.0.1:6379",
		},
	}
	p := NewProver(config)
	p.proofModel.DropProofTable()
	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			prover := NewProver(config)
			prover.proofModel.CreateProofTable()
			for {
				var batchWitnesses []*witness.BatchWitness
				var err error
				batchWitnesses, err = prover.FetchBatchWitness()
				if errors.Is(err, utils.DbErrNotFound) {
					fmt.Println("there is no published status witness in db, so quit")
					fmt.Println("prover run finish...")
					return
				}
				if errors.Is(err, redis.Nil) {
					fmt.Println("There is no task left in task queue")
					fmt.Println("prover run finish...")
					return
				}
				if err != nil {
					fmt.Println("get batch witness failed: ", err.Error())
					time.Sleep(10 * time.Second)
					continue
				}
				// time.Sleep(1 * time.Second)
				for _, batchWitness := range batchWitnesses {
					var row = &Proof{
						ProofInfo:               "testproof",
						BatchNumber:             batchWitness.Height,
						CexAssetListCommitments: string("testcexAssetListCommitments"),
						AccountTreeRoots:        string("testaccountTreeRoots"),
						BatchCommitment:         string("testbatchCommitment"),
						AssetsCount:             0,
					}
					err = prover.proofModel.CreateProof(row)
					if err != nil {
						fmt.Printf("create blockProof of height %d failed\n", batchWitness.Height)
						panic(err.Error())
					}
					err = prover.witnessModel.UpdateBatchWitnessStatus(batchWitness, witness.StatusFinished)
					if err != nil {
						fmt.Println("update witness error:", err.Error())
						panic(err.Error())
					}
				}
			}			
		}(i)
	}
	wg.Wait()
	counts, _ := witnessTable.GetRowCounts()
	if counts[1] != 0 || counts[2] != 0 || counts[3] != 100000 {
		fmt.Println("witness table row counts: ", counts)
		t.Fatal("get row counts failed")
	}
	proofCount, _ := p.proofModel.GetRowCounts()
	if proofCount != 100000 {
		t.Fatal("proof count not equal to 100000")
	}
}