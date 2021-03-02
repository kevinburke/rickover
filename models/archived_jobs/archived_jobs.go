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
func Get(ctx context.Context, id types.PrefixUUID) (*newmodels.ArchivedJob, error) {
	aj, err := newmodels.DB.GetArchivedJob(ctx, id)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	aj.ID.Prefix = Prefix
	return &aj, nil
}

// GetRetry attempts to retrieve the job attempts times before giving up,
// sleeping 50 milliseconds between each attempt.
func GetRetry(ctx context.Context, id types.PrefixUUID, attempts int) (*newmodels.ArchivedJob, error) {
	var err error
	for i := 0; i < attempts; i++ {
		var job *newmodels.ArchivedJob
		job, err = Get(ctx, id)
		if err == nil {
			return job, nil
		}
		if err == ErrNotFound || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, err
}
