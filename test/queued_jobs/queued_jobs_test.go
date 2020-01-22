package test_queued_jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kevinburke/go-types"
	"github.com/kevinburke/rickover/models/jobs"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/services"
	"github.com/kevinburke/rickover/test"
	"github.com/kevinburke/rickover/test/factory"
	"github.com/lib/pq"
)

var empty = json.RawMessage([]byte("{}"))

var sampleJob = newmodels.CreateJobParams{
	Name:             "echo",
	DeliveryStrategy: newmodels.DeliveryStrategyAtLeastOnce,
	Attempts:         3,
	Concurrency:      1,
}

func TestAll(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	t.Run("Parallel", func(t *testing.T) {
		// Parallel tests go here
		t.Run("TestEnqueueUnknownJobTypeErrNoRows", testEnqueueUnknownJobTypeErrNoRows)
		t.Run("TestNonexistentReturnsErrNoRows", testNonexistentReturnsErrNoRows)
		t.Run("TestDeleteNonexistentJobReturnsErrNoRows", testDeleteNonexistentJobReturnsErrNoRows)
		t.Run("TestGetQueuedJob", testGetQueuedJob)
		t.Run("TestDeleteQueuedJob", testDeleteQueuedJob)
		t.Run("TestAcquireReturnsCorrectValues", testAcquireReturnsCorrectValues)
		t.Run("TestEnqueueNoData", testEnqueueNoData)
		t.Run("EnqueueWithExistingArchivedJobFails", testEnqueueWithExistingArchivedJobFails)
	})
}

func testEnqueueNoData(t *testing.T) {
	t.Parallel()
	id := types.GenerateUUID("jobname_")
	j := newmodels.CreateJobParams{
		Name:             id.String(),
		DeliveryStrategy: newmodels.DeliveryStrategyAtLeastOnce,
		Attempts:         7,
		Concurrency:      1,
	}
	_, err := jobs.Create(j)
	test.AssertNotError(t, err, "")

	expiresAt := types.NullTime{Valid: false}
	runAfter := time.Now().UTC()

	qjid := types.GenerateUUID("job_")
	_, err = queued_jobs.Enqueue(newmodels.EnqueueJobParams{
		ID:        qjid,
		Name:      j.Name,
		RunAfter:  runAfter,
		ExpiresAt: expiresAt,
		Data:      []byte{},
	})
	test.AssertError(t, err, "")
	switch terr := err.(type) {
	case *pq.Error:
		test.AssertEquals(t, terr.Message, "invalid input syntax for type json")
	default:
		t.Fatalf("Expected a pq.Error, got %#v", terr)
	}
}

func newParams(id types.PrefixUUID, name string, runAfter time.Time, expiresAt types.NullTime, data []byte) newmodels.EnqueueJobParams {
	return newmodels.EnqueueJobParams{
		ID:        id,
		Name:      name,
		RunAfter:  runAfter,
		ExpiresAt: expiresAt,
		Data:      data,
	}
}

func TestEnqueueJobExists(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	_, err := jobs.Create(sampleJob)
	test.AssertNotError(t, err, "")

	expiresAt := types.NullTime{Valid: false}
	runAfter := time.Now().UTC()

	_, err = queued_jobs.Enqueue(newParams(factory.JobId, "echo", runAfter, expiresAt, empty))
	test.AssertNotError(t, err, "")
	_, err = queued_jobs.Enqueue(newParams(factory.JobId, "echo", runAfter, expiresAt, empty))
	test.AssertError(t, err, "")
	switch terr := err.(type) {
	case *pq.Error:
		test.AssertEquals(t, terr.Code, pq.ErrorCode("23505"))
		test.AssertEquals(t, terr.Table, "queued_jobs")
		test.AssertEquals(t, terr.Message,
			`duplicate key value violates unique constraint "queued_jobs_pkey"`)
	default:
		t.Fatalf("Expected a pq.Error, got %#v", terr)
	}
}

func testEnqueueUnknownJobTypeErrNoRows(t *testing.T) {
	t.Parallel()

	expiresAt := types.NullTime{Valid: false}
	runAfter := time.Now().UTC()
	_, err := queued_jobs.Enqueue(newParams(factory.JobId, "unknownJob", runAfter, expiresAt, empty))
	test.AssertError(t, err, "")
	test.AssertEquals(t, err.Error(), "Job type unknownJob does not exist or the job with that id has already been archived")
}

func testEnqueueWithExistingArchivedJobFails(t *testing.T) {
	t.Parallel()
	_, qj := factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	err := services.HandleStatusCallback(context.Background(), qj.ID, qj.Name, newmodels.ArchivedJobStatusSucceeded, qj.Attempts, true)
	test.AssertNotError(t, err, "")
	expiresAt := types.NullTime{Valid: false}
	runAfter := time.Now().UTC()
	_, err = queued_jobs.Enqueue(newParams(qj.ID, qj.Name, runAfter, expiresAt, empty))
	test.AssertError(t, err, "")
	test.AssertEquals(t, err.Error(), "Job type "+qj.Name+" does not exist or the job with that id has already been archived")
}

func testNonexistentReturnsErrNoRows(t *testing.T) {
	t.Parallel()
	id, _ := types.NewPrefixUUID("job_a9173b65-7714-42b4-85f2-8336f6d12180")
	_, err := queued_jobs.Get(context.Background(), id)
	test.AssertEquals(t, err, queued_jobs.ErrNotFound)
}

func testGetQueuedJob(t *testing.T) {
	t.Parallel()
	_, qj := factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	gotQj, err := queued_jobs.Get(context.Background(), qj.ID)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, gotQj.ID.String(), qj.ID.String())
}

func testDeleteQueuedJob(t *testing.T) {
	t.Parallel()
	_, qj := factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	err := queued_jobs.Delete(context.Background(), qj.ID)
	test.AssertNotError(t, err, "")
}

func testDeleteNonexistentJobReturnsErrNoRows(t *testing.T) {
	t.Parallel()
	err := queued_jobs.Delete(context.Background(), factory.RandomId("job_"))
	test.AssertEquals(t, err, queued_jobs.ErrNotFound)
}

func TestEnqueue(t *testing.T) {
	defer test.TearDown(t)
	qj := factory.CreateQueuedJob(t, factory.EmptyData)
	test.AssertEquals(t, qj.ID.String(), "job_6740b44e-13b9-475d-af06-979627e0e0d6")
	test.AssertEquals(t, qj.Name, "echo")
	test.AssertEquals(t, qj.Attempts, int16(7))
	test.AssertEquals(t, qj.Status, newmodels.JobStatusQueued)

	diff := time.Since(qj.RunAfter)
	test.Assert(t, diff < 100*time.Millisecond, "")

	diff = time.Since(qj.CreatedAt)
	test.Assert(t, diff < 100*time.Millisecond, "")

	diff = time.Since(qj.UpdatedAt)
	test.Assert(t, diff < 100*time.Millisecond, "")
}

func TestDataRoundtrip(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	_, err := jobs.Create(sampleJob)
	test.AssertNotError(t, err, "")

	type RemoteAccounts struct {
		ID string
	}

	type User struct {
		ID             string
		Balance        uint64
		CreatedAt      time.Time
		RemoteAccounts RemoteAccounts
		Pickups        []string
	}
	user := &User{
		ID:        "usr_123",
		Balance:   uint64(365),
		CreatedAt: time.Now().UTC(),
		RemoteAccounts: RemoteAccounts{
			ID: "rem_123",
		},
		Pickups: []string{"pik_123", "pik_234"},
	}

	expiresAt := types.NullTime{Valid: false}
	runAfter := time.Now().UTC()
	var d json.RawMessage
	d, err = json.Marshal(user)
	test.AssertNotError(t, err, "")
	qj, err := queued_jobs.Enqueue(newParams(factory.JobId, "echo", runAfter, expiresAt, d))
	test.AssertNotError(t, err, "")

	gotQj, err := queued_jobs.Get(context.Background(), qj.ID)
	test.AssertNotError(t, err, "")

	var u User
	err = json.Unmarshal(gotQj.Data, &u)
	test.AssertNotError(t, err, "expected to be able to convert data to a User object")
	test.AssertEquals(t, u.ID, "usr_123")
	test.AssertEquals(t, u.Balance, uint64(365))
	test.AssertEquals(t, u.RemoteAccounts.ID, "rem_123")
	test.AssertEquals(t, u.Pickups[0], "pik_123")
	test.AssertEquals(t, u.Pickups[1], "pik_234")
	test.AssertEquals(t, len(u.Pickups), 2)

	diff := time.Since(u.CreatedAt)
	test.Assert(t, diff < 100*time.Millisecond, "")
}

func testAcquireReturnsCorrectValues(t *testing.T) {
	t.Parallel()
	job, qj := factory.CreateUniqueQueuedJob(t, factory.EmptyData)

	gotQj, err := queued_jobs.Acquire(context.TODO(), job.Name, 1)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, gotQj.ID.String(), qj.ID.String())
	test.AssertEquals(t, gotQj.Status, newmodels.JobStatusInProgress)
}

func TestAcquireTwoThreads(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	job, _ := factory.CreateUniqueQueuedJob(t, factory.EmptyData)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var err1, err2 error
	var gotQj1, gotQj2 *newmodels.QueuedJob
	resultCh := make(chan struct{}, 1)
	go func() {
		gotQj1, err1 = queued_jobs.Acquire(ctx, job.Name, 1)
		resultCh <- struct{}{}
	}()
	go func() {
		gotQj2, err2 = queued_jobs.Acquire(ctx, job.Name, 1)
		resultCh <- struct{}{}
	}()

	<-resultCh
	<-resultCh
	if gotQj1 != nil {
		fmt.Printf("%#v\n", *gotQj1)
	}
	if gotQj2 != nil {
		fmt.Printf("%#v\n", *gotQj2)
	}
	test.Assert(t, err1 == sql.ErrNoRows || err2 == sql.ErrNoRows, fmt.Sprintf("expected one error to be ErrNoRows, got %v %v", err1, err2))
	test.Assert(t, gotQj1 != nil || gotQj2 != nil, "expected one job to be acquired")
}

func TestAcquireDoesntGetFutureJob(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)

	_, err := jobs.Create(sampleJob)
	test.AssertNotError(t, err, "")

	expiresAt := types.NullTime{Valid: false}
	runAfter := time.Now().UTC().Add(20 * time.Millisecond)
	qj, err := queued_jobs.Enqueue(newParams(factory.JobId, "echo", runAfter, expiresAt, empty))
	test.AssertNotError(t, err, "")
	_, err = queued_jobs.Acquire(context.TODO(), qj.Name, 1)
	test.AssertEquals(t, err, sql.ErrNoRows)
}

func TestAcquireDoesntGetInProgressJob(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)

	_, err := jobs.Create(sampleJob)
	test.AssertNotError(t, err, "")

	expiresAt := types.NullTime{Valid: false}
	runAfter := time.Now().UTC()
	qj, err := queued_jobs.Enqueue(newParams(factory.JobId, "echo", runAfter, expiresAt, empty))
	test.AssertNotError(t, err, "")
	qj, err = queued_jobs.Acquire(context.TODO(), qj.Name, 1)
	test.AssertNotError(t, err, "")
	test.AssertDeepEquals(t, qj.ID, factory.JobId)

	_, err = queued_jobs.Acquire(context.TODO(), qj.Name, 1)
	test.AssertEquals(t, err, sql.ErrNoRows)
}

func TestDecrementDecrements(t *testing.T) {
	defer test.TearDown(t)
	qj := factory.CreateQueuedJob(t, factory.EmptyData)
	qj, err := queued_jobs.Decrement(qj.ID, 7, time.Now().Add(1*time.Minute))
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, qj.Attempts, int16(6))
	test.AssertBetween(t, int64(time.Until(qj.RunAfter)), int64(59*time.Second), int64(1*time.Minute))
}

func TestDecrementErrNoRowsWrongAttempts(t *testing.T) {
	defer test.TearDown(t)
	qj := factory.CreateQueuedJob(t, factory.EmptyData)
	_, err := queued_jobs.Decrement(qj.ID, 1, time.Now())
	test.AssertEquals(t, err, sql.ErrNoRows)
}

func TestCountAll(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	allCount, readyCount, err := queued_jobs.CountReadyAndAll(context.Background())
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, allCount, 0)
	test.AssertEquals(t, readyCount, 0)

	factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	allCount, readyCount, err = queued_jobs.CountReadyAndAll(context.Background())
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, allCount, 3)
	test.AssertEquals(t, readyCount, 3)
}

func TestCountByStatus(t *testing.T) {
	defer test.TearDown(t)
	job, _ := factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	factory.CreateQueuedJobOnly(t, job.Name, factory.EmptyData)
	factory.CreateQueuedJobOnly(t, job.Name, factory.EmptyData)
	factory.CreateAtMostOnceJob(t, factory.EmptyData)
	m, err := queued_jobs.GetCountsByStatus(context.Background(), newmodels.JobStatusQueued)
	test.AssertNotError(t, err, "")
	test.Assert(t, len(m) >= 2, "expected at least 2 queued jobs in the database")
	test.AssertEquals(t, m[job.Name], int64(3))
	test.AssertEquals(t, m["at-most-once"], int64(1))
}

func TestOldInProgress(t *testing.T) {
	defer test.TearDown(t)
	ctx := context.Background()
	_, qj1 := factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	_, qj2 := factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	_, err := queued_jobs.Acquire(ctx, qj1.Name, 1)
	test.AssertNotError(t, err, "")
	_, err = queued_jobs.Acquire(ctx, qj2.Name, 1)
	test.AssertNotError(t, err, "")
	jobs, err := queued_jobs.GetOldInProgressJobs(ctx, time.Now().UTC().Add(40*time.Millisecond))
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, len(jobs), 2)
	if jobs[0].ID.String() == qj1.ID.String() {
		test.AssertEquals(t, jobs[1].ID.String(), qj2.ID.String())
	} else {
		test.AssertEquals(t, jobs[1].ID.String(), qj1.ID.String())
	}
	jobs, err = queued_jobs.GetOldInProgressJobs(ctx, time.Now().UTC().Add(-1*time.Second))
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, len(jobs), 0)
}
