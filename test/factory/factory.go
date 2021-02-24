// Package factory contains helpers for instantiating tests.
package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	log "github.com/inconshreveable/log15"
	"github.com/kevinburke/go-types"
	uuid "github.com/kevinburke/go.uuid"
	"github.com/kevinburke/rickover/models/archived_jobs"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/services"
	"github.com/kevinburke/rickover/test"
	"github.com/lib/pq"
)

var EmptyData = json.RawMessage([]byte("{}"))

var JobId types.PrefixUUID

func init() {
	id, _ := types.NewPrefixUUID("job_6740b44e-13b9-475d-af06-979627e0e0d6")
	JobId = id
}

type RandomData struct {
	Foo []string `json:"foo"`
	Baz uint8    `json:"baz"`
}

var RD = &RandomData{
	Foo: []string{"bar", "pik_345"},
	Baz: uint8(17),
}

var SampleJob = newmodels.CreateJobParams{
	Name:             "echo",
	DeliveryStrategy: newmodels.DeliveryStrategyAtLeastOnce,
	Attempts:         7,
	Concurrency:      1,
}

var SampleAtMostOnceJob = newmodels.CreateJobParams{
	Name:             "at-most-once",
	DeliveryStrategy: newmodels.DeliveryStrategyAtMostOnce,
	Attempts:         1,
	Concurrency:      5,
}

// RandomId returns a random UUID with the given prefix.
func RandomId(prefix string) types.PrefixUUID {
	id := uuid.NewV4()
	return types.PrefixUUID{
		UUID:   id,
		Prefix: prefix,
	}
}

func CreateJob(t testing.TB, j newmodels.CreateJobParams) newmodels.Job {
	test.SetUp(t)
	if j.DeliveryStrategy == "" {
		j.DeliveryStrategy = newmodels.DeliveryStrategyAtLeastOnce
	}
	if j.Attempts == 0 {
		j.Attempts = 11
	}
	job, err := newmodels.DB.CreateJob(context.Background(), j)
	test.AssertNotError(t, err, "")
	return job
}

// CreateQueuedJob creates a job and a queued job with the given JSON data, and
// returns the created queued job.
func CreateQueuedJob(t testing.TB, data json.RawMessage) *newmodels.QueuedJob {
	t.Helper()
	_, qj := createJobAndQueuedJob(t, SampleJob, data, false)
	return qj
}

// Like the above but with unique ID's and job names
func CreateUniqueQueuedJob(t testing.TB, data json.RawMessage) (*newmodels.Job, *newmodels.QueuedJob) {
	id := types.GenerateUUID("jobname_")
	j := newmodels.CreateJobParams{
		Name:             id.String()[:len(id.Prefix)+8],
		DeliveryStrategy: newmodels.DeliveryStrategyAtLeastOnce,
		Attempts:         7,
		Concurrency:      1,
	}
	return createJobAndQueuedJob(t, j, data, true)
}

func CreateQueuedJobOnly(t testing.TB, name string, data json.RawMessage) {
	t.Helper()
	expiresAt := types.NullTime{Valid: false}
	runAfter := time.Now().UTC()
	err := queued_jobs.EnqueueFast(newmodels.EnqueueJobFastParams{
		Name: name, RunAfter: runAfter, ExpiresAt: expiresAt, Data: data,
	})
	test.AssertNotError(t, err, "")
}

// CreateQJ creates a job with a random name, and a random UUID.
func CreateQJ(t testing.TB) *newmodels.QueuedJob {
	t.Helper()
	test.SetUp(t)
	jobname := RandomId("jobtype")
	job, err := newmodels.DB.CreateJob(context.Background(), newmodels.CreateJobParams{
		Name:             jobname.String(),
		Attempts:         11,
		Concurrency:      3,
		DeliveryStrategy: newmodels.DeliveryStrategyAtLeastOnce,
	})
	test.AssertNotError(t, err, "create job failed")
	now := time.Now().UTC()
	expires := types.NullTime{
		Time:  now.Add(5 * time.Minute),
		Valid: true,
	}
	dat, err := json.Marshal(RD)
	test.AssertNotError(t, err, "marshaling RD")
	qj, err := services.Enqueue(context.Background(), newmodels.DB,
		newmodels.EnqueueJobParams{
			ID: RandomId("job_"), Name: job.Name, RunAfter: now, ExpiresAt: expires, Data: dat,
		})
	test.AssertNotError(t, err, "create job failed")
	return &qj
}

func CreateArchivedJob(t *testing.T, data json.RawMessage, status newmodels.ArchivedJobStatus) *newmodels.ArchivedJob {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, qj := createJobAndQueuedJob(t, SampleJob, data, false)
	aj, err := archived_jobs.Create(ctx, qj.ID, qj.Name, newmodels.ArchivedJobStatusSucceeded, qj.Attempts)
	test.AssertNotError(t, err, "")
	err = queued_jobs.DeleteRetry(ctx, qj.ID, 3)
	test.AssertNotError(t, err, "")
	return aj
}

// CreateAtMostOnceJob creates a queued job that can be run at most once.
func CreateAtMostOnceJob(t *testing.T, data json.RawMessage) (*newmodels.Job, *newmodels.QueuedJob) {
	t.Helper()
	return createJobAndQueuedJob(t, SampleAtMostOnceJob, data, false)
}

func createJobAndQueuedJob(t testing.TB, j newmodels.CreateJobParams, data json.RawMessage, randomId bool) (*newmodels.Job, *newmodels.QueuedJob) {
	test.SetUp(t)
	job, err := newmodels.DB.CreateJob(context.Background(), newmodels.CreateJobParams{
		Name:             j.Name,
		DeliveryStrategy: j.DeliveryStrategy,
		Attempts:         j.Attempts,
		Concurrency:      j.Concurrency,
	})
	if err != nil {
		switch dberr := err.(type) {
		case *pq.Error:
			if dberr.Code == "23505" {
			} else {
				test.AssertNotError(t, err, "")
			}
		default:
			test.AssertNotError(t, err, "")
		}
	}

	expiresAt := types.NullTime{Valid: false}
	runAfter := time.Now().UTC()
	var id types.PrefixUUID
	if randomId {
		id = RandomId("job_")
	} else {
		id = JobId
	}
	qj, err := services.Enqueue(context.Background(), newmodels.DB, newmodels.EnqueueJobParams{
		ID: id, Name: j.Name, RunAfter: runAfter, ExpiresAt: expiresAt,
		Data: data,
	})
	test.AssertNotError(t, err, fmt.Sprintf("Error creating queued job %s (job name %s)", id, j.Name))
	return &job, &qj
}

// Processor returns a simple JobProcessor, with a client pointing at the given
// URL, password set to "password" and various sleeps set to 0.
func Processor(url string) *services.JobProcessor {
	logger := log.New()
	logger.SetHandler(log.DiscardHandler())
	handler := services.NewDownstreamHandler(logger, url, "password")
	return &services.JobProcessor{
		Logger:  logger,
		Timeout: 200 * time.Millisecond,
		Handler: handler,
	}
}
