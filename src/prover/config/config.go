package config

type Config struct {
	MysqlDataSource string
	DbSuffix        string
	ZkKeyName []string
	AssetsCountTiers []int
	BatchCount      int
}
