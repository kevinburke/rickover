// Logic for interacting with the "jobs" table.
package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	dberror "github.com/kevinburke/go-dberror"
	"github.com/kevinburke/rickover/models"
	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/lib/pq"
)

func init() {
	dberror.RegisterConstraint(concurrencyConstraint)
	dberror.RegisterConstraint(attemptsConstraint)
}

var insertJobStmt *sql.Stmt
var getJobStmt *sql.Stmt
var getAllJobStmt *sql.Stmt

// Setup prepares all database queries in this package.
func Setup() (err error) {
	if !db.Connected() {
		return errors.New("jobs: no database connection, bailing")
	}

	if insertJobStmt != nil {
		return
	}

	insertJobStmt, err = db.Conn.Prepare(fmt.Sprintf(`-- jobs.Create
INSERT INTO jobs (%s) VALUES ($1, $2, $3, $4) RETURNING %s`,
		fields(false), fields(true)))
	if err != nil {
		return err
	}

	getJobStmt, err = db.Conn.Prepare(fmt.Sprintf(`-- jobs.Get
SELECT %s
FROM jobs
WHERE name = $1`, fields(true)))
	if err != nil {
		return err
	}

	getAllJobStmt, err = db.Conn.Prepare(fmt.Sprintf(`-- jobs.Get
SELECT %s
FROM jobs`, fields(true)))
	if err != nil {
		return err
	}

	return
}

func Create(job newmodels.Job) (*newmodels.Job, error) {
	job, err := newmodels.DB.CreateJob(context.Background(), newmodels.CreateJobParams{
		Name:             job.Name,
		DeliveryStrategy: job.DeliveryStrategy,
		Attempts:         job.Attempts,
		Concurrency:      job.Concurrency,
	})
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

func GetAll() ([]*models.Job, error) {
	rows, err := getAllJobStmt.Query()
	if err != nil {
		return []*models.Job{}, err
	}
	defer rows.Close()
	var jobs []*models.Job
	for rows.Next() {
		job := new(models.Job)
		if err := rows.Scan(args(job)...); err != nil {
			return jobs, err
		}
		jobs = append(jobs, job)
	}
	err = rows.Err()
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

func fields(includeCreatedAt bool) string {
	if includeCreatedAt {
		return `name,
delivery_strategy,
attempts,
concurrency,
created_at`
	} else {
		return `name,
delivery_strategy,
attempts,
concurrency`
	}
}

func args(job *models.Job) []interface{} {
	return []interface{}{
		&job.Name,
		&job.DeliveryStrategy,
		&job.Attempts,
		&job.Concurrency,
		&job.CreatedAt,
	}
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
