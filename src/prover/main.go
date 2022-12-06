package main

import (
	"encoding/json"
	"flag"
	"github.com/binance/zkmerkle-proof-of-solvency/src/prover/config"
	"github.com/binance/zkmerkle-proof-of-solvency/src/prover/prover"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"io/ioutil"
)

func main() {
	proverConfig := &config.Config{}
	content, err := ioutil.ReadFile("config/config.json")
	if err != nil {
		panic(err.Error())
	}
	err = json.Unmarshal(content, proverConfig)
	if err != nil {
		panic(err.Error())
	}
	remotePasswdConfig := flag.String("remote_password_config", "", "fetch password from aws secretsmanager")
	rerun := flag.Bool("rerun", false, "flag which indicates rerun proof generation")
	flag.Parse()
	if *remotePasswdConfig != "" {
		s, err := utils.GetPostgresqlSource(proverConfig.PostgresDataSource, *remotePasswdConfig)
		if err != nil {
			panic(err.Error())
		}
		proverConfig.PostgresDataSource = s
	}
	prover := prover.NewProver(proverConfig)
	prover.Run(*rerun)
}
