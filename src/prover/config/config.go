package config

type Config struct {
	MysqlDataSource string
	DbSuffix        string
	Redis           struct {
		Host     string
		Type     string
		Password string
	}
	ZkKeyName string
}
