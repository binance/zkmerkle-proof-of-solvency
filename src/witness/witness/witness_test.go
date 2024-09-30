package witness

import (
	"testing"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"bytes"
	"fmt"
	"sync"
	"gorm.io/gorm/logger"
	"os"
	"time"
	"log"
	// "github.com/klauspost/compress/s2"
)

func TestWitnessModel(t *testing.T) {
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

	witnessTable := NewWitnessModel(db, "test")
	err = witnessTable.CreateBatchWitnessTable()
	if err != nil {
		t.Errorf("error: %s\n", err.Error())
	}
	largeArray := bytes.Repeat([]byte{'a'}, 178000)
	
	datas := make([]BatchWitness, 100)
	for i := 0; i < 100; i++ {
		w := BatchWitness{
			Height: int64(i),
			Status: 0,
			WitnessData: string(largeArray),
		}
		datas[i] = w
	}
	startTime := time.Now()
	err = witnessTable.CreateBatchWitness(datas)
	if err != nil {
		t.Errorf("error: %s\n", err.Error())
	}
	endTime := time.Now()
	fmt.Println("create witness data time is ", endTime.Sub(startTime))

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			witnesses, err := witnessTable.GetAndUpdateBatchesWitnessByStatus(0, 1, 7)
			fmt.Println("worker:", index, "len is ", len(witnesses), "err is ", err)
		}(i)
	}
	wg.Wait()
}