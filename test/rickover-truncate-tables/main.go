package main

import (
	"context"
	"log"

	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/setup"
	"github.com/kevinburke/rickover/test"
)

func main() {
	if err := setup.DB(context.Background(), db.DefaultConnection, 1); err != nil {
		log.Fatal(err)
	}
	if err := test.TruncateTables(nil); err != nil {
		log.Fatal(err)
	}
}
