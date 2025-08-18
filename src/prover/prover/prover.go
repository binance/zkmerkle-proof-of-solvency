package prover

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/binance/zkmerkle-proof-of-solvency/circuit"
	"github.com/binance/zkmerkle-proof-of-solvency/src/prover/config"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/binance/zkmerkle-proof-of-solvency/src/witness/witness"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/constraint/solver"
	"github.com/consensys/gnark/frontend"
	"github.com/redis/go-redis/v9"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Prover struct {
	witnessModel witness.WitnessModel
	proofModel   ProofModel
	redisCli     *redis.Client

	VerifyingKey groth16.VerifyingKey
	ProvingKey   groth16.ProvingKey
	SessionName   []string
	AssetsCountTiers    []int
	R1cs          constraint.ConstraintSystem

	CurrentSnarkParamsInUse int
	TaskQueueName string
}

func NewProver(config *config.Config) *Prover {
	db, err := gorm.Open(mysql.Open(config.MysqlDataSource))
	if err != nil {
		panic(err.Error())
	}
	// Set up the redis client.
	redisCli := redis.NewClient(&redis.Options{
		Addr:    config.Redis.Host,
		Password: config.Redis.Password,
	})
	taskQueueName := "por_batch_task_queue_" + config.DbSuffix

	prover := Prover{
		witnessModel: witness.NewWitnessModel(db, config.DbSuffix),
		proofModel:   NewProofModel(db, config.DbSuffix),
		redisCli:     redisCli,
		SessionName:  config.ZkKeyName,
		AssetsCountTiers:  config.AssetsCountTiers,
		CurrentSnarkParamsInUse: 0,
		TaskQueueName: taskQueueName,
	}

	// std.RegisterHints()
	solver.RegisterHint(circuit.IntegerDivision)
	return &prover
}

func (p *Prover) fetchTasksByRedis() (int, error) {
	var ctx = context.Background()
	batchHeightStr, err := p.redisCli.BRPop(ctx, 10*time.Second, p.TaskQueueName).Result()
	if err != nil {
		return -1, err
	}

	batchHeight, err := strconv.Atoi(batchHeightStr[1])
	if err != nil {
		return -1, err
	}
	return batchHeight, nil
}

func (p *Prover) FetchBatchWitness() ([]*witness.BatchWitness, error) {
	batchHeight, err := p.fetchTasksByRedis()
	if err != nil {
		return nil, err
	}

	// Fetch unproved block witness.
	for {
		blockWitnesses, err := p.witnessModel.GetAndUpdateBatchesWitnessByHeight(batchHeight, witness.StatusPublished, witness.StatusReceived)
		if err == utils.DbErrQueryInterrupted || err == utils.DbErrQueryTimeout {
			fmt.Println("get batch witness timeout, retry...:", err.Error())
			time.Sleep(1 * time.Second)
			continue
		}
		if err != nil {
			return nil, err
		}
		return blockWitnesses, nil
	}
}

func (p *Prover) FetchBatchWitnessForRerun() ([]*witness.BatchWitness, error) {
	var blockWitness *witness.BatchWitness
	var err error
	for {
		blockWitness, err = p.witnessModel.GetLatestBatchWitnessByStatus(witness.StatusReceived)
		if err == utils.DbErrQueryInterrupted || err == utils.DbErrQueryTimeout {
			fmt.Println("get latest batch witness by status timeout, retry...:", err.Error())
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}

	if err == utils.DbErrNotFound {
		for {
			blockWitness, err = p.witnessModel.GetLatestBatchWitnessByStatus(witness.StatusPublished)
			if err == utils.DbErrQueryInterrupted || err == utils.DbErrQueryTimeout {
				fmt.Println("get latest batch witness by status timeout, retry...:", err.Error())
				time.Sleep(1 * time.Second)
				continue
			}
			break
		}
	}
	if err != nil {
		return nil, err
	}
	blockWitnesses := make([]*witness.BatchWitness, 1)
	blockWitnesses[0] = blockWitness
	return blockWitnesses, nil
}

func (p *Prover) Run(flag bool) {
	p.proofModel.CreateProofTable()
	for {
		var batchWitnesses []*witness.BatchWitness
		var err error
		if !flag {
			// when the task is removed from redis queue,
			// 1. if prover crash before updating witness status to pending, or
			// 2. if prover crash before generating proof,
			// then the offline rerun mechanism will be triggered to handle this situation.
			batchWitnesses, err = p.FetchBatchWitness()
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
		} else {
			batchWitnesses, err = p.FetchBatchWitnessForRerun()
			if errors.Is(err, utils.DbErrNotFound) {
				fmt.Println("there is no received status witness in db, so quit")
				fmt.Println("prover rerun finish...")
				return
			}
			if err != nil {
				fmt.Println("something wrong happened, err is ", err.Error())
				return
			}
		}

		for _, batchWitness := range batchWitnesses {
			witnessForCircuit := utils.DecodeBatchWitness(batchWitness.WitnessData)
			cexAssetListCommitments := make([][]byte, 2)
			cexAssetListCommitments[0] = witnessForCircuit.BeforeCEXAssetsCommitment
			cexAssetListCommitments[1] = witnessForCircuit.AfterCEXAssetsCommitment
			accountTreeRoots := make([][]byte, 2)
			accountTreeRoots[0] = witnessForCircuit.BeforeAccountTreeRoot
			accountTreeRoots[1] = witnessForCircuit.AfterAccountTreeRoot
			cexAssetListCommitmentsSerial, err := json.Marshal(cexAssetListCommitments)
			if err != nil {
				fmt.Println("marshal cex asset list failed: ", err.Error())
				return
			}
			accountTreeRootsSerial, err := json.Marshal(accountTreeRoots)
			if err != nil {
				fmt.Println("marshal account tree root failed: ", err.Error())
				return
			}
			proof, assetsCount, err := p.GenerateAndVerifyProof(witnessForCircuit, batchWitness.Height)
			if err != nil {
				fmt.Println("generate and verify proof error:", err.Error())
				return
			}
			var buf bytes.Buffer
			_, err = proof.WriteRawTo(&buf)
			if err != nil {
				fmt.Println("proof serialize failed")
				return
			}
			proofBytes := buf.Bytes()

			// Check the existence of block proof.
			for {
				_, err = p.proofModel.GetProofByBatchNumber(batchWitness.Height)
				if err == utils.DbErrQueryInterrupted || err == utils.DbErrQueryTimeout {
					fmt.Println("get proof by batch number timeout, retry...:", err.Error())
					time.Sleep(1 * time.Second)
					continue
				}
				break
			}
			if err == nil {
				fmt.Printf("blockProof of height %d exists\n", batchWitness.Height)
				err = p.witnessModel.UpdateBatchWitnessStatus(batchWitness, witness.StatusFinished)
				if err != nil {
					fmt.Println("update witness error:", err.Error())
				}
				continue
			}

			var row = &Proof{
				ProofInfo:               base64.StdEncoding.EncodeToString(proofBytes),
				BatchNumber:             batchWitness.Height,
				CexAssetListCommitments: string(cexAssetListCommitmentsSerial),
				AccountTreeRoots:        string(accountTreeRootsSerial),
				BatchCommitment:         base64.StdEncoding.EncodeToString(witnessForCircuit.BatchCommitment),
				AssetsCount:             assetsCount,
			}
			err = p.proofModel.CreateProof(row)
			if err != nil {
				fmt.Printf("create blockProof of height %d failed\n", batchWitness.Height)
				return
			}
			err = p.witnessModel.UpdateBatchWitnessStatus(batchWitness, witness.StatusFinished)
			if err != nil {
				fmt.Println("update witness error:", err.Error())
			}
		}
	}
}

func (p *Prover) GenerateAndVerifyProof(
	batchWitness *utils.BatchCreateUserWitness,
	batchNumber int64,
) (proof groth16.Proof, assetsCount int, err error) {
	startTime := time.Now().UnixMilli()
	fmt.Println("begin to generate proof for batch: ", batchNumber)
	circuitWitness, _ := circuit.SetBatchCreateUserCircuitWitness(batchWitness)
	// Lazy load r1cs, proving key and verifying key.
	p.LoadSnarkParamsOnce(len(circuitWitness.CreateUserOps[0].Assets))
	verifyWitness := circuit.NewVerifyBatchCreateUserCircuit(batchWitness.BatchCommitment)
	witness, err := frontend.NewWitness(circuitWitness, ecc.BN254.ScalarField())
	if err != nil {
		return proof, 0, err
	}

	vWitness, err := frontend.NewWitness(verifyWitness, ecc.BN254.ScalarField(), frontend.PublicOnly())
	if err != nil {
		return proof, 0, err
	}
	proof, err = groth16.Prove(p.R1cs, p.ProvingKey, witness)
	if err != nil {
		return proof, 0, err
	}
	endTime := time.Now().UnixMilli()
	fmt.Println("proof generation cost ", endTime-startTime, " ms")

	err = groth16.Verify(proof, p.VerifyingKey, vWitness)
	if err != nil {
		return proof, 0, err
	}
	endTime2 := time.Now().UnixMilli()
	fmt.Println("proof verification cost ", endTime2-endTime, " ms")
	return proof, len(circuitWitness.CreateUserOps[0].Assets), nil
}

func (p *Prover) LoadSnarkParamsOnce(targerAssetsCount int) {
	if targerAssetsCount == p.CurrentSnarkParamsInUse {
		return
	}
	
	index := -1
	for i, v :=  range p.AssetsCountTiers {
		if targerAssetsCount == v {
			index = i
			break
		}
	}
	if index == -1 {
		panic("the assets count is not in the config file")
	}
	// Load r1cs, proving key and verifying key.
	s := time.Now()
	fmt.Println("begin loading r1cs of ", targerAssetsCount, " assets")
	loadR1csChan := make(chan bool)
	go func() {
		for {

			select {
			case <-loadR1csChan:
				fmt.Println("load r1cs finished...... quit")
				return
			case <-time.After(time.Second * 10):
				runtime.GC()
			}
		}
	}()

	p.R1cs = groth16.NewCS(ecc.BN254)

	r1csFromFile, err := os.ReadFile(p.SessionName[index] + ".r1cs")
	if err != nil {
		panic("r1cs file load error..." + err.Error())
	}
	buf := bytes.NewBuffer(r1csFromFile)
	n, err := p.R1cs.ReadFrom(buf)
	if err != nil {
		panic("r1cs read error..." + err.Error())
	}
	fmt.Println("r1cs read size is ", n)
	loadR1csChan <- true
	runtime.GC()
	et := time.Now()
	fmt.Println("finish loading r1cs.... the time cost is ", et.Sub(s))
	
	// read proving and verifying keys
	fmt.Println("begin loading proving key of ", targerAssetsCount, " assets")
	s = time.Now()
	pkFromFile, err := os.ReadFile(p.SessionName[index] + ".pk")
	if err != nil {
		panic("provingKey file load error:" + err.Error())
	}
	buf = bytes.NewBuffer(pkFromFile)
	p.ProvingKey = groth16.NewProvingKey(ecc.BN254)
	n, err = p.ProvingKey.UnsafeReadFrom(buf)
	if err != nil {
		panic("provingKey loading error:" + err.Error())
	}
	fmt.Println("proving key read size is ", n)
	et = time.Now()
	fmt.Println("finish loading proving key... the time cost is ", et.Sub(s))
	
	fmt.Println("begin loading verifying key of ", targerAssetsCount, " assets")
	s = time.Now()
	vkFromFile, err := os.ReadFile(p.SessionName[index] + ".vk")
	if err != nil {
		panic("verifyingKey file load error:" + err.Error())
	}
	buf = bytes.NewBuffer(vkFromFile)
	p.VerifyingKey = groth16.NewVerifyingKey(ecc.BN254)
	n, err = p.VerifyingKey.ReadFrom(buf)
	if err != nil {
		panic("verifyingKey loading error:" + err.Error())
	}
	fmt.Println("verifying key read size is ", n)
	et = time.Now()
	fmt.Println("finish loading verifying key.. the time cost is ", et.Sub(s))
	p.CurrentSnarkParamsInUse = targerAssetsCount
}
