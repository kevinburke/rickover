package newmodels

import (
	"errors"

	"github.com/kevinburke/rickover/models/db"
)

var DB *Queries

func Setup() error {
	if !db.Connected() {
		return errors.New("newmodels: no database connection, bailing")
	}

	DB = New(db.Conn)
	return nil
}
