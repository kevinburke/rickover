// Logic for interacting with the "jobs" table.
package jobs

import (
	"context"
	"database/sql"
	"time"

	dberror "github.com/kevinburke/go-dberror"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/lib/pq"
)

func init() {
	dberror.RegisterConstraint(concurrencyConstraint)
	dberror.RegisterConstraint(attemptsConstraint)
}

func Create(params newmodels.CreateJobParams) (*newmodels.Job, error) {
	job, err := newmodels.DB.CreateJob(context.Background(), params)
	if err != nil {
		err = dberror.GetError(err)
	}
	return &job, err
}

// Get a job by its name.
func Get(name string) (*newmodels.Job, error) {
	job, err := newmodels.DB.GetJob(context.Background(), name)
	if err != nil {
		return nil, dberror.GetError(err)
	}
	return &job, nil
}

func GetAll() ([]newmodels.Job, error) {
	jobs, err := newmodels.DB.GetAllJobs(context.TODO())
	return jobs, err
}

// GetRetry attempts to get the job `attempts` times before giving up.
func GetRetry(name string, attempts uint8) (job *newmodels.Job, err error) {
	for i := uint8(0); i < attempts; i++ {
		job, err = Get(name)
		if err == nil || err == sql.ErrNoRows {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return
}

var concurrencyConstraint = &dberror.Constraint{
	Name: "jobs_concurrency_check",
	GetError: func(e *pq.Error) *dberror.Error {
		return &dberror.Error{
			Message:    "Concurrency must be a positive number",
			Constraint: e.Constraint,
			Table:      e.Table,
			Severity:   e.Severity,
			Detail:     e.Detail,
		}
	},
}

var attemptsConstraint = &dberror.Constraint{
	Name: "jobs_attempts_check",
	GetError: func(e *pq.Error) *dberror.Error {
		return &dberror.Error{
			Message:    "Please set a greater-than-zero number of attempts",
			Constraint: e.Constraint,
			Table:      e.Table,
			Severity:   e.Severity,
			Detail:     e.Detail,
		}
	},
}
