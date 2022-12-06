package config

type Config struct {
	PostgresDataSource string
	DbSuffix           string
	TreeDB             struct {
		Driver string
		Option struct {
			Addr string
		}
	}
}
