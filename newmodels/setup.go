package newmodels

import (
	"context"
	"errors"

	"github.com/kevinburke/rickover/models/db"
)

var DB *Queries

func Setup(ctx context.Context) error {
	if !db.Connected() {
		return errors.New("newmodels: no database connection, bailing")
	}

	var err error
	DB, err = Prepare(ctx, db.Conn)
	return err
}
