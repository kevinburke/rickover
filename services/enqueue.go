package services

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
)

// Enqueue creates a new queued job with the given ID and fields. A
// sql.ErrNoRows will be returned if the `name` does not exist in the jobs
// table. Otherwise the QueuedJob will be returned.
func Enqueue(ctx context.Context, qs *newmodels.Queries, params newmodels.EnqueueJobParams) (newmodels.QueuedJob, error) {
	var err error
	for i := 0; i < 3; i++ {
		var qj newmodels.QueuedJob
		qj, err = qs.EnqueueJob(ctx, params)
		if err == nil {
			qj.ID.Prefix = queued_jobs.Prefix
			return qj, nil
		}
		if err == sql.ErrNoRows {
			e := &queued_jobs.UnknownOrArchivedError{
				Err: fmt.Sprintf("Job type %q does not exist or the job with that id has already been archived",
					params.Name),
			}
			return newmodels.QueuedJob{}, e
		}
		if db.IsRetryable(err) {
			continue
		}
		return newmodels.QueuedJob{}, err
	}
	return newmodels.QueuedJob{}, err
}
