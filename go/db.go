package main

import (
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var (
	dbHosts = []string{"192.168.0.12", "192.168.0.13"}
	dbs     = make([]*sqlx.DB, len(dbHosts))
)

func initDatabase() (err error) {

	for i, host := range dbHosts {
		port := os.Getenv("ISUCON_DB_PORT")
		if port == "" {
			port = "3306"
		}
		_, err := strconv.Atoi(port)
		if err != nil {
			panic(fmt.Sprintf("failed to convert DB port number from ISUCON_DB_PORT environment variable into int: %v", err))
		}
		user := os.Getenv("ISUCON_DB_USER")
		if user == "" {
			user = "isucon"
		}
		password := os.Getenv("ISUCON_DB_PASSWORD")
		if password == "" {
			password = "isucon"
		}
		dbname := os.Getenv("ISUCON_DB_NAME")
		if dbname == "" {
			dbname = "isuride"
		}
		dbConfig := mysql.NewConfig()
		dbConfig.User = user
		dbConfig.Passwd = password
		dbConfig.Addr = net.JoinHostPort(host, port)
		dbConfig.Net = "tcp"
		dbConfig.DBName = dbname
		dbConfig.ParseTime = true
		dbConfig.InterpolateParams = true
		dbConfig.MultiStatements = true

		dbs[i], err = sqlx.Connect("mysql", dbConfig.FormatDSN())
		if err != nil {
			panic(err)
		}
	}
	return nil
}

func database() *sqlx.DB {
	return dbs[0]
}

// func chairDatabase() *sqlx.DB {
// 	return dbs[1]
// }
