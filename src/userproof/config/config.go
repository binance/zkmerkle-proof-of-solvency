package config

type Config struct {
	MysqlDataSource string
	UserDataFile    string
	DbSuffix        string
	TreeDB          struct {
		Driver string
		Option struct {
			Addr string
		}
	}
}
