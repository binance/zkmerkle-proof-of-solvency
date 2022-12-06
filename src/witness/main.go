package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"github.com/binance/zkmerkle-proof-of-solvency/src/witness/config"
	"github.com/binance/zkmerkle-proof-of-solvency/src/witness/witness"
	"io/ioutil"
	"math/big"
)

func GenerateFakeCexAssetsInfo() []utils.CexAssetInfo {
	cexAssetsInfoList := make([]utils.CexAssetInfo, utils.AssetCounts)
	for i := 0; i < utils.AssetCounts; i++ {
		cexAssetsInfoList[i].BasePrice = uint64(i + 1)
	}
	return cexAssetsInfoList
}

func GenerateFakeAccounts(counts uint32, cexAssetsInfo []utils.CexAssetInfo) []utils.AccountInfo {

	accounts := make([]utils.AccountInfo, counts)
	for i := uint32(0); i < counts; i++ {
		assets := make([]utils.AccountAsset, utils.AssetCounts)
		accounts[i].TotalEquity = new(big.Int).SetInt64(0)
		accounts[i].TotalDebt = new(big.Int).SetInt64(0)
		for j := 0; j < utils.AssetCounts; j++ {
			assets[j].Equity = uint64(j*2 + 1)
			assets[j].Debt = uint64(j + 1)
			accounts[i].TotalEquity = new(big.Int).Add(accounts[i].TotalEquity,
				new(big.Int).Mul(new(big.Int).SetUint64(assets[j].Equity), new(big.Int).SetUint64(cexAssetsInfo[j].BasePrice)))
			accounts[i].TotalDebt = new(big.Int).Add(accounts[i].TotalDebt,
				new(big.Int).Mul(new(big.Int).SetUint64(assets[j].Debt), new(big.Int).SetUint64(cexAssetsInfo[j].BasePrice)))
		}
		accounts[i].AccountIndex = uint32(i)
		accounts[i].Assets = assets
	}
	return accounts
}

func main() {
	remotePasswdConfig := flag.String("remote_password_config", "", "fetch password from aws secretsmanager")
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
		s, err := utils.GetPostgresqlSource(witnessConfig.PostgresDataSource, *remotePasswdConfig)
		if err != nil {
			panic(err.Error())
		}
		witnessConfig.PostgresDataSource = s
	}

	accounts, cexAssetsInfo, err := utils.ParseUserDataSet(witnessConfig.UserDataFile)
	fmt.Println("account counts", len(accounts))
	if err != nil {
		panic(err.Error())
	}
	accountTree, err := utils.NewAccountTree(witnessConfig.TreeDB.Driver, witnessConfig.TreeDB.Option.Addr)
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("account tree init height is ", accountTree.LatestVersion())
	fmt.Printf("account tree root is %x\n", accountTree.Root())

	//var accountsNumber uint32 = 1000000
	//cexAssetsInfo := GenerateFakeCexAssetsInfo()
	//accounts := GenerateFakeAccounts(accountsNumber, cexAssetsInfo)
	witnessService := witness.NewWitness(accountTree, uint32(len(accounts)), accounts, cexAssetsInfo, witnessConfig)
	witnessService.Run()
	fmt.Println("witness service run finished...")
}
