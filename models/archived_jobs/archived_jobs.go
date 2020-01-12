// Logic for interacting with the "archived_jobs" table.
package archived_jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kevinburke/go-dberror"
	"github.com/kevinburke/go-types"
	"github.com/kevinburke/rickover/models"
	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/newmodels"
)

const Prefix = "job_"

// ErrNotFound indicates that the archived job was not found.
var ErrNotFound = errors.New("archived_jobs: job not found")

var createStmt *sql.Stmt
var getStmt *sql.Stmt

// Setup prepares all database statements.
func Setup() (err error) {
	if !db.Connected() {
		return errors.New("archived_jobs: no DB connection was established, can't query")
	}

	if createStmt != nil {
		return
	}

	query := fmt.Sprintf(`-- archived_jobs.Create
INSERT INTO archived_jobs (%s) 
SELECT id, $2, $4, $3, data, expires_at
FROM queued_jobs 
WHERE id=$1
AND name=$2
RETURNING %s`, insertFields(), fields())
	createStmt, err = db.Conn.Prepare(query)
	if err != nil {
		return err
	}

	query = fmt.Sprintf(`-- archived_jobs.Get
SELECT %s
FROM archived_jobs
WHERE id = $1`, fields())
	getStmt, err = db.Conn.Prepare(query)
	return
}

// Create an archived job with the given id, status, and attempts. Assumes that
// the job already exists in the queued_jobs table; the `data` field is copied
// from there. If the job does not exist, queued_jobs.ErrNotFound is returned.
func Create(id types.PrefixUUID, name string, status newmodels.ArchivedJobStatus, attempt int16) (*newmodels.ArchivedJob, error) {
	aj, err := newmodels.DB.CreateArchivedJob(context.Background(), newmodels.CreateArchivedJobParams{
		ID:       id,
		Name:     name,
		Status:   status,
		Attempts: attempt,
	})
	if err != nil {
		return nil, dberror.GetError(err)
	}
	return &aj, nil
}

// Get returns the archived job with the given id, or sql.ErrNoRows if it's
// not present.
func Get(id types.PrefixUUID) (*newmodels.ArchivedJob, error) {
	aj, err := newmodels.DB.GetArchivedJob(context.Background(), id)
	if err != nil {
		return nil, dberror.GetError(err)
	}
	return &aj, nil
}

// GetRetry attempts to retrieve the job attempts times before giving up.
func GetRetry(id types.PrefixUUID, attempts uint8) (job *newmodels.ArchivedJob, err error) {
	for i := uint8(0); i < attempts; i++ {
		job, err = Get(id)
		if err == nil || err == ErrNotFound {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return
}

func insertFields() string {
	return `id,
	name,
	attempts,
	status,
	data,
	expires_at`
}

func fields() string {
	return fmt.Sprintf(`'%s' || id,
	name,
	attempts,
	status,
	data,
	created_at,
	expires_at`, Prefix)
}

func args(aj *models.ArchivedJob, byteptr *[]byte) []interface{} {
	return []interface{}{
		&aj.ID,
		&aj.Name,
		&aj.Attempts,
		&aj.Status,
		// can't scan into Data because of https://github.com/golang/go/issues/13905
		byteptr,
		&aj.CreatedAt,
		&aj.ExpiresAt,
	}
}
