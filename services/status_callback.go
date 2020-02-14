// Mediation layer between the server and database queries.
//
// Logic that's not related to validating request input/turning errors into
// HTTP responses should go here.
package services

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	log "github.com/inconshreveable/log15"
	metrics "github.com/kevinburke/go-simple-metrics"
	"github.com/kevinburke/go-types"
	"github.com/kevinburke/rickover/models/archived_jobs"
	"github.com/kevinburke/rickover/models/jobs"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/lib/pq"
)

// HandleStatusCallback updates a queued job with the provided status and
// the attempts remaining. Likely the job will either be inserted into
// archived_jobs and removed from queued_jobs, or the job will have its
// attempts counter decremented in queued_jobs.
//
// This can return an error if any of the following happens: the archived_job
// already exists, the queued job no longer exists by the time you attempt to
// delete it, the number of attempts for the queued job don't match up with the
// passed in value (slow)
func HandleStatusCallback(ctx context.Context, logger log.Logger, id types.PrefixUUID, name string, status newmodels.ArchivedJobStatus, attempt int16, retryable bool) error {
	switch status {
	case newmodels.ArchivedJobStatusSucceeded:
		err := createAndDelete(ctx, logger, id, name, newmodels.ArchivedJobStatusSucceeded, attempt)
		if err != nil {
			metrics.Increment("archived_job.create.success.error")
		} else {
			metrics.Increment(fmt.Sprintf("archived_job.create.%s.success", name))
			metrics.Increment("archived_job.create.success")
			metrics.Increment("archived_job.create")
		}
		return err
	case newmodels.ArchivedJobStatusFailed:
		err := handleFailedCallback(ctx, logger, id, name, attempt, retryable)
		if err != nil {
			metrics.Increment("archived_job.create.failed.error")
		} else {
			metrics.Increment(fmt.Sprintf("archived_job.create.%s.failed", name))
			metrics.Increment("archived_job.create.failed")
			metrics.Increment("archived_job.create")
		}
		return err
	default:
		return fmt.Errorf("services: unknown job status: %s", status)
	}
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	switch terr := err.(type) {
	case *pq.Error:
		return terr.Code == "23505"
	default:
		return false
	}
}

// createAndDelete creates an archived job, deletes the queued job, and returns
// any errors.
func createAndDelete(ctx context.Context, logger log.Logger, id types.PrefixUUID, name string, status newmodels.ArchivedJobStatus, attempt int16) error {
	start := time.Now()
	_, err := archived_jobs.Create(ctx, id, name, status, attempt)
	metrics.Time("archived_job.create.latency", time.Since(start))
	if err != nil {
		switch {
		case isUniqueViolation(err):
			// Some other thread beat us to it. Don't return an error, just
			// fall through and try to delete the record.
			logger.Info("Could not create archived job because "+
				"it was already present. Deleting the queued job.", "id", id.String(), "status", status)
		default:
			return err
		}
	}
	start = time.Now()
	err = queued_jobs.DeleteRetry(ctx, id, 3)
	metrics.Time("queued_job.delete.latency", time.Since(start))
	return err
}

// getRunAfter gets the time this job should run after, given the current
// attempt number and the attempts remaining.
func getRunAfter(totalAttempts, remainingAttempts int16) time.Time {
	backoff := totalAttempts - remainingAttempts
	return time.Now().UTC().Add(time.Duration(math.Pow(2, float64(backoff))) * time.Second)
}

var ErrFailedDecrement = fmt.Errorf("could not decrement queued job counter; job may have been archived or attempt number may not match the database")

func handleFailedCallback(ctx context.Context, logger log.Logger, id types.PrefixUUID, name string, attempt int16, retryable bool) error {
	remainingAttempts := attempt - 1
	if !retryable || remainingAttempts == 0 {
		return createAndDelete(ctx, logger, id, name, newmodels.ArchivedJobStatusFailed, remainingAttempts)
	}
	job, err := jobs.GetRetry(ctx, name, 3)
	if err != nil {
		return err
	}
	if job.DeliveryStrategy == newmodels.DeliveryStrategyAtMostOnce {
		return createAndDelete(ctx, logger, id, name, newmodels.ArchivedJobStatusFailed, remainingAttempts)
	} else {
		// Try the job again. Note the database decrements the attempt counter
		start := time.Now()
		runAfter := getRunAfter(job.Attempts, remainingAttempts)
		_, err := queued_jobs.Decrement(ctx, id, attempt, runAfter)
		metrics.Time("queued_jobs.decrement.latency", time.Since(start))
		// Possible the queued job exists but the attempt number doesn't line up
		if err == sql.ErrNoRows {
			return ErrFailedDecrement
		}
		return err
	}
}
