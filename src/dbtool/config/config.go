package config

type Config struct {
	MysqlDataSource string
	DbSuffix        string
	TreeDB          struct {
		Driver string
		Option struct {
			Addr string
		}
	}
	Redis           struct {
		Host     	string
		Password  	string
	}
}
