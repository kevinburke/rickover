// Logic for interacting with the "jobs" table.
package jobs

import (
	"context"
	"database/sql"
	"time"

	"github.com/kevinburke/rickover/newmodels"
)

func Create(params newmodels.CreateJobParams) (*newmodels.Job, error) {
	job, err := newmodels.DB.CreateJob(context.Background(), params)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// Get a job by its name.
func Get(name string) (*newmodels.Job, error) {
	job, err := newmodels.DB.GetJob(context.Background(), name)
	if err != nil {
		return nil, err
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
