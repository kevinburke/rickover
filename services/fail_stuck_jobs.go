package services

import (
	"context"
	"time"

	log "github.com/inconshreveable/log15"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
)

// ArchiveStuckJobs marks as failed any queued jobs with an updated_at
// timestamp older than the olderThan value.
func ArchiveStuckJobs(ctx context.Context, logger log.Logger, olderThan time.Duration) error {
	var olderThanTime time.Time
	if olderThan >= 0 {
		olderThanTime = time.Now().Add(-1 * olderThan)
	} else {
		olderThanTime = time.Now().Add(olderThan)
	}
	getCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	jobs, err := queued_jobs.GetOldInProgressJobs(getCtx, olderThanTime)
	cancel()
	if err != nil {
		return err
	}
	for _, qj := range jobs {
		// bad to cancel this halfway through, give it time to run regardless fo
		// the server state.
		handleCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		err = HandleStatusCallback(handleCtx, logger, qj.ID, qj.Name, newmodels.ArchivedJobStatusFailed, qj.Attempts, true)
		if err == nil {
			logger.Info("found stuck job and marked it as failed", "id", qj.ID.String())
		} else {
			// We don't want to return an error here since there may easily be
			// race/idempotence errors with a stuck job watcher. If it errors
			// we'll grab it with the next cron.
			logger.Error("found stuck job but could not process it", "id", qj.ID.String(), "err", err)
		}
	}
	return nil
}

// WatchStuckJobs polls the queued_jobs table for stuck jobs (defined as
// in-progress jobs that haven't been updated in oldDuration time), and marks
// them as failed.
func WatchStuckJobs(ctx context.Context, logger log.Logger, interval time.Duration, olderThan time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		err := ArchiveStuckJobs(ctx, logger, olderThan)
		if err != nil {
			logger.Error("could not archive stuck jobs", "err", err)
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}
