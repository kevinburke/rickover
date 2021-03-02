// Package queued_jobs contains logic for interacting with the "queued_jobs"
// table.
package queued_jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kevinburke/go-types"
	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/newmodels"
)

const Prefix = "job_"

// ErrNotFound indicates that the job was not found.
var ErrNotFound = errors.New("queued_jobs: job not found")

// UnknownOrArchivedError is raised when the job type is unknown or the job has
// already been archived. It's unfortunate we can't distinguish these, but more
// important to minimize the total number of queries to the database.
type UnknownOrArchivedError struct {
	Err string
}

func (e *UnknownOrArchivedError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Err
}

// StuckJobLimit is the maximum number of stuck jobs to fetch in one database
// query.
var StuckJobLimit = 100

// Enqueue creates a new queued job with the given ID and fields. A
// sql.ErrNoRows will be returned if the `name` does not exist in the jobs
// table. Otherwise the QueuedJob will be returned.
//
// Deprecated: use services.Enqueue instead.
func Enqueue(params newmodels.EnqueueJobParams) (*newmodels.QueuedJob, error) {
	qj, err := newmodels.DB.EnqueueJob(context.TODO(), params)
	if err != nil {
		if err == sql.ErrNoRows {
			e := &UnknownOrArchivedError{
				Err: fmt.Sprintf("Job type %s does not exist or the job with that id has already been archived", params.Name),
			}
			return nil, e
		}
		return nil, err
	}
	qj.ID.Prefix = Prefix
	return &qj, err
}

func EnqueueFast(params newmodels.EnqueueJobFastParams) error {
	err := newmodels.DB.EnqueueJobFast(context.TODO(), params)
	if err == nil {
		return nil
	}
	if err == sql.ErrNoRows {
		e := &UnknownOrArchivedError{
			Err: fmt.Sprintf("Job type %s does not exist or the job with that id has already been archived", params.Name),
		}
		return e
	}
	return err
}

// Get the queued job with the given id. Returns the job, or an error. If no
// record could be found, the error will be `queued_jobs.ErrNotFound`.
func Get(ctx context.Context, id types.PrefixUUID) (*newmodels.QueuedJob, error) {
	qj, err := newmodels.DB.GetQueuedJob(ctx, id)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	qj.ID.Prefix = Prefix
	return &qj, nil
}

func GetAttempts(ctx context.Context, id types.PrefixUUID) (int16, error) {
	attempts, err := newmodels.DB.GetQueuedJobAttempts(ctx, id)
	if err == sql.ErrNoRows {
		return -1, ErrNotFound
	}
	return attempts, nil
}

// GetRetry attempts to retrieve the job attempts times before giving up.
func GetRetry(ctx context.Context, id types.PrefixUUID, attempts int) (job *newmodels.QueuedJob, err error) {
	for i := 0; i < attempts; i++ {
		job, err = Get(ctx, id)
		if err == nil || err == ErrNotFound || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			break
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return
}

// Delete deletes the given queued job. Returns nil if the job was deleted
// successfully. If no job exists to be deleted, ErrNotFound is returned.
func Delete(ctx context.Context, id types.PrefixUUID) error {
	num, err := newmodels.DB.DeleteQueuedJob(ctx, id)
	if err != nil {
		return err
	}
	if len(num) == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteRetry attempts to Delete the item `attempts` times.
func DeleteRetry(ctx context.Context, id types.PrefixUUID, attempts int) error {
	for i := 0; i < attempts; i++ {
		err := Delete(ctx, id)
		if err == nil || err == ErrNotFound || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}
	return nil
}

var useOldMethod = false
var useRecursiveMethod = true

// Acquire a queued job with the given name that's able to run now. Returns
// the queued job and a boolean indicating whether the SELECT query found
// a row, or a generic error/sql.ErrNoRows if no jobs are available.
func Acquire(ctx context.Context, name string, workerID int) (*newmodels.QueuedJob, error) {
	if useOldMethod {
		qj, err := newmodels.DB.OldAcquireJob(ctx, name)
		if err != nil {
			return nil, err
		}
		qj.ID.Prefix = Prefix
		return &qj, nil
	}
	tx, err := db.Conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if useRecursiveMethod {
		var i newmodels.QueuedJob
		err = tx.QueryRowContext(ctx, `
WITH RECURSIVE lock_candidates (n) AS (
    -- Pick a bunch of candidate jobs from the database
    SELECT * FROM (
        SELECT 0, auto_id, id,
        row_number() over (order by auto_id) as rownumber, false as locked, 0::bigint as locked_row_id
        FROM queued_jobs
        WHERE status='queued' 
            AND name = $1
            AND run_after < now()
        ORDER BY auto_id
        LIMIT 100
    ) t1
  UNION ALL (
    -- Try to lock each one in turn. The first time we recurse, n+1 = 1, so we
    -- try to lock the first row in the CASE WHEN statement.
    -- Second time, n+1 = 2, we try to lock the second row in the CASE WHEN
    -- statement.
    WITH t2 AS (
        SELECT lock_candidates.n+1, lock_candidates.auto_id, lock_candidates.id,
            lock_candidates.rownumber,
            CASE WHEN lock_candidates.n+1 = lock_candidates.rownumber
                THEN (pg_try_advisory_xact_lock(lock_candidates.auto_id))
                ELSE false
            END AS locked
        FROM queued_jobs
        -- Join so we only check the rows that were pulled by the first query
        INNER JOIN lock_candidates ON queued_jobs.auto_id = lock_candidates.auto_id
        WHERE lock_candidates.n < 100
    ), t2_and_locked_row AS (
        -- Put the auto_id of the locked row at the end of every row, we use
        -- this to make t3 easier
        SELECT t2.*, COALESCE((SELECT auto_id c FROM t2 WHERE locked = true LIMIT 1), 0) locked_row_id FROM t2
    )
    -- Return either the single locked row OR all of the non-locked rows.
    SELECT *
    FROM t2_and_locked_row t3
    WHERE (locked_row_id > 0 AND t3.auto_id = locked_row_id) OR (
        locked_row_id = 0
    )
  )
)
UPDATE queued_jobs
SET status = 'in-progress', updated_at = now()
WHERE id = (SELECT id FROM lock_candidates where locked_row_id > 0 LIMIT 1)
AND status = 'queued'
RETURNING id, name, attempts, run_after, expires_at, created_at, updated_at, status, data, auto_id
`, name).Scan(
			&i.ID,
			&i.Name,
			&i.Attempts,
			&i.RunAfter,
			&i.ExpiresAt,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.Status,
			&i.Data,
			&i.AutoID,
		)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		i.ID.Prefix = Prefix
		return &i, nil
	}
	qs := newmodels.DB.WithTx(tx)
	defer qs.Close()
	var qjid types.PrefixUUID
	for i := 0; i < 5; i++ {
		qjid, err = qs.AcquireJob(ctx, name)
		if err != sql.ErrNoRows {
			break
		}
	}
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	qj, err := qs.MarkInProgress(ctx, qjid)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	qj.ID.Prefix = Prefix
	return &newmodels.QueuedJob{ID: qjid}, nil
}

// Decrement decrements the attempts counter for an existing job, and sets
// its status back to 'queued'. If the queued job does not exist, or the
// attempts counter in the database does not match the passed in attempts
// value, sql.ErrNoRows will be returned.
//
// attempts: The current value of the `attempts` column, the returned attempts
// value will be this number minus 1.
func Decrement(ctx context.Context, id types.PrefixUUID, attempts int16, runAfter time.Time) (*newmodels.QueuedJob, error) {
	qj, err := newmodels.DB.DecrementQueuedJob(ctx, newmodels.DecrementQueuedJobParams{
		ID:       id,
		Attempts: attempts,
		RunAfter: runAfter,
	})
	if err != nil {
		return nil, err
	}
	qj.ID.Prefix = Prefix
	return &qj, nil
}

// GetOldInProgressJobs finds queued in-progress jobs with an updated_at
// timestamp older than olderThan. A maximum of StuckJobLimit jobs will be
// returned.
func GetOldInProgressJobs(ctx context.Context, olderThan time.Time) ([]newmodels.QueuedJob, error) {
	jobs, err := newmodels.DB.GetOldInProgressJobs(ctx, olderThan)
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		jobs[i].ID.Prefix = Prefix
	}
	return jobs, nil
}

// CountReadyAndAll returns the total number of queued and ready jobs in the
// table.
func CountReadyAndAll(ctx context.Context) (all int, ready int, err error) {
	result, err := newmodels.DB.CountReadyAndAll(ctx)
	if err != nil {
		return 0, 0, err
	}
	return int(result.All), int(result.Ready), nil
}

// GetCountsByStatus returns a map with each job type as the key, followed by
// the number of <status> jobs it has. For example:
//
// "echo": 5,
// "remind-assigned-driver": 7,
func GetCountsByStatus(ctx context.Context, status newmodels.JobStatus) (map[string]int64, error) {
	counts, err := newmodels.DB.GetQueuedCountsByStatus(ctx, status)
	if err != nil {
		return nil, err
	}
	mp := make(map[string]int64, len(counts))
	for i := range counts {
		mp[counts[i].Name] = counts[i].Count
	}
	return mp, nil
}
