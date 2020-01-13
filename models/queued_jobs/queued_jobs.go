// Logic for interacting with the "queued_jobs" table.
package queued_jobs

import (
	"context"
	"database/sql"
	"encoding/json"
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
// dberror.Error will be returned if Postgres returns a constraint failure -
// job exists, job name unknown, &c. A sql.ErrNoRows will be returned if the
// `name` does not exist in the jobs table. Otherwise the QueuedJob will be
// returned.
func Enqueue(id types.PrefixUUID, name string, runAfter time.Time, expiresAt sql.NullTime, data json.RawMessage) (*newmodels.QueuedJob, error) {
	qj, err := newmodels.DB.EnqueueJob(context.TODO(), newmodels.EnqueueJobParams{
		ID:        id,
		Name:      name,
		RunAfter:  runAfter,
		ExpiresAt: expiresAt,
		Data:      data,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			e := &UnknownOrArchivedError{
				Err: fmt.Sprintf("Job type %s does not exist or the job with that id has already been archived", name),
			}
			return nil, e
		}
		return nil, dberror.GetError(err)
	}
	return &qj, err
}

// Get the queued job with the given id. Returns the job, or an error. If no
// record could be found, the error will be `queued_jobs.ErrNotFound`.
func Get(id types.PrefixUUID) (*newmodels.QueuedJob, error) {
	qj, err := newmodels.DB.GetQueuedJob(context.Background(), id)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, dberror.GetError(err)
	}
	return &qj, nil
}

// GetRetry attempts to retrieve the job attempts times before giving up.
func GetRetry(id types.PrefixUUID, attempts uint8) (job *newmodels.QueuedJob, err error) {
	for i := uint8(0); i < attempts; i++ {
		job, err = Get(id)
		if err == nil || err == ErrNotFound {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return
}

// Delete deletes the given queued job. Returns nil if the job was deleted
// successfully. If no job exists to be deleted, ErrNotFound is returned.
func Delete(id types.PrefixUUID) error {
	num, err := newmodels.DB.DeleteQueuedJob(context.Background(), id)
	if err != nil {
		return err
	}
	if len(num) == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteRetry attempts to Delete the item `attempts` times.
func DeleteRetry(id types.PrefixUUID, attempts uint8) error {
	for i := uint8(0); i < attempts; i++ {
		err := Delete(id)
		if err == nil || err == ErrNotFound {
			return err
		}
	}
	return nil
}

// Acquire a queued job with the given name that's able to run now. Returns
// the queued job and a boolean indicating whether the SELECT query found
// a row, or a generic error/sql.ErrNoRows if no jobs are available.
func Acquire(name string) (*newmodels.QueuedJob, error) {
	tx, err := db.Conn.BeginTx(context.TODO(), nil)
	if err != nil {
		return nil, err
	}
	qs := newmodels.DB.WithTx(tx)

	qj, err := qs.AcquireJob(context.TODO(), name)
	if err != nil {
		tx.Rollback()
		err = dberror.GetError(err)
		return nil, err
	}
	qj2, err := qs.MarkInProgress(context.TODO(), qj.ID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &qj2, nil
}

// Decrement decrements the attempts counter for an existing job, and sets
// its status back to 'queued'. If the queued job does not exist, or the
// attempts counter in the database does not match the passed in attempts
// value, sql.ErrNoRows will be returned.
//
// attempts: The current value of the `attempts` column, the returned attempts
// value will be this number minus 1.
func Decrement(id types.PrefixUUID, attempts int16, runAfter time.Time) (*newmodels.QueuedJob, error) {
	qj, err := newmodels.DB.DecrementQueuedJob(context.Background(), newmodels.DecrementQueuedJobParams{
		ID:       id,
		Attempts: attempts,
		RunAfter: runAfter,
	})
	if err != nil {
		return nil, dberror.GetError(err)
	}
	return &qj, nil
}

// GetOldInProgressJobs finds queued in-progress jobs with an updated_at
// timestamp older than olderThan. A maximum of StuckJobLimit jobs will be
// returned.
func GetOldInProgressJobs(olderThan time.Time) ([]newmodels.QueuedJob, error) {
	jobs, err := newmodels.DB.GetOldInProgressJobs(context.Background(), olderThan)
	if err != nil {
		return nil, dberror.GetError(err)
	}
	return jobs, nil
}

// CountReadyAndAll returns the total number of queued and ready jobs in the
// table.
func CountReadyAndAll() (allCount int, readyCount int, err error) {
	result, err := newmodels.DB.CountReadyAndAll(context.TODO())
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
func GetCountsByStatus(status newmodels.JobStatus) (map[string]int64, error) {
	counts, err := newmodels.DB.GetQueuedCountsByStatus(context.Background(), status)
	if err != nil {
		return nil, err
	}
	fmt.Println("counts", counts)
	return nil, nil
}

func insertFields() string {
	return `id,
	name,
	attempts,
	run_after,
	expires_at,
	status,
	data`
}

func fields() string {
	return fmt.Sprintf(`'%s' || id,
	name,
	attempts,
	run_after,
	expires_at,
	status,
	data,
	created_at,
	updated_at`, Prefix)
}

func args(qj *models.QueuedJob) []interface{} {
	return []interface{}{
		&qj.ID,
		&qj.Name,
		&qj.Attempts,
		&qj.RunAfter,
		&qj.ExpiresAt,
		&qj.Status,
		&qj.Data,
		&qj.CreatedAt,
		&qj.UpdatedAt,
	}
}
