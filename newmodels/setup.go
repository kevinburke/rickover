package newmodels

import (
	"context"
	"errors"

	"github.com/kevinburke/rickover/models/db"
)

// DefaultServer relies on this being initialized (and not overwritten)
var DB = new(Queries)

func (q *Queries) Connected() bool {
	return q.db != nil || q.tx != nil
}

func Setup(ctx context.Context) error {
	if !db.Connected() {
		return errors.New("newmodels: no database connection, bailing")
	}

	qs, err := Prepare(ctx, db.Conn)
	if err != nil {
		return err
	}
	*DB = *qs
	return nil
}
