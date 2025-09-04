package utils

import "errors"

var (
	DbErrSqlOperation  = errors.New("unknown sql operation error")
	DbErrNotFound      = errors.New("sql: no rows in result set")
	DbErrTableNotFound   = errors.New("sql: table not found")
	DbErrQueryTimeout  = errors.New("sql: query timeout")
	DbErrQueryInterrupted = errors.New("sql: query interrupted")
)
