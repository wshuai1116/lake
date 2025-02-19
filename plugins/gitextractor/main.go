package main

import (
	"context"
	"flag"
	"strings"

	"github.com/merico-dev/lake/plugins/gitextractor/models"
	"github.com/merico-dev/lake/plugins/gitextractor/parser"
	"github.com/merico-dev/lake/plugins/gitextractor/store"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	url := flag.String("url", "", "-url ")
	proxy := flag.String("proxy", "", "-proxy")
	id := flag.String("id", "", "-id")
	output := flag.String("output", "", "-output")
	db := flag.String("db", "", "-db")
	flag.Parse()
	var storage models.Store
	var err error
	if *output != "" {
		storage, err = store.NewCsvStore(*output)
		if err != nil {
			panic(err)
		}
	}
	if *db != "" {
		database, err := gorm.Open(mysql.Open(*db))
		if err != nil {
			panic(err)
		}
		storage = store.NewDatabase(database)
	}
	defer storage.Close()
	ctx := context.Background()
	p := parser.NewLibGit2(storage)
	if strings.HasPrefix(*url, "http") {
		err = p.RemoteRepo(ctx, *url, *id, *proxy)
		if err != nil {
			panic(err)
		}
	}
	if strings.HasPrefix(*url, "/") {
		err = p.LocalRepo(ctx, *url, *id)
		if err != nil {
			panic(err)
		}
	}
}
