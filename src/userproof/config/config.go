package config

type Config struct {
	PostgresDataSource string
	UserDataFile       string
	DbSuffix           string
	TreeDB             struct {
		Driver string
		Option struct {
			Addr string
		}
	}
}
