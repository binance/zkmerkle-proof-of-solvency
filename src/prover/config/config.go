package config

type Config struct {
	MysqlDataSource string
	DbSuffix        string
	Redis           struct {
		Host     	string
		Password  	string
	}
	ZkKeyName []string
	AssetsCountTiers []int
}
