// Logic for interacting with the "archived_jobs" table.
package archived_jobs

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/kevinburke/go-types"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
)

const Prefix = "job_"

// ErrNotFound indicates that the archived job was not found.
var ErrNotFound = errors.New("archived_jobs: job not found")

// Create an archived job with the given id, status, and attempts. Assumes that
// the job already exists in the queued_jobs table; the `data` field is copied
// from there. If the job does not exist, queued_jobs.ErrNotFound is returned.
func Create(ctx context.Context, id types.PrefixUUID, name string, status newmodels.ArchivedJobStatus, attempt int16) (*newmodels.ArchivedJob, error) {
	aj, err := newmodels.DB.CreateArchivedJob(ctx, newmodels.CreateArchivedJobParams{
		ID:       id,
		Name:     name,
		Status:   status,
		Attempts: attempt,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, queued_jobs.ErrNotFound
		}
		return nil, err
	}
	aj.ID.Prefix = Prefix
	return &aj, nil
}

// Get returns the archived job with the given id, or sql.ErrNoRows if it's
// not present.
func Get(id types.PrefixUUID) (*newmodels.ArchivedJob, error) {
	aj, err := newmodels.DB.GetArchivedJob(context.Background(), id)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	aj.ID.Prefix = Prefix
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
