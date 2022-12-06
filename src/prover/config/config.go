package config

type Config struct {
	PostgresDataSource string
	DbSuffix           string
	Redis              struct {
		Host     string
		Type     string
		Password string
	}
	ZkKeyName string
}
