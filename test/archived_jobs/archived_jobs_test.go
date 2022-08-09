package test_archived_jobs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kevinburke/rickover/models/archived_jobs"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/test"
	"github.com/kevinburke/rickover/test/factory"
	"github.com/lib/pq"
)

func TestAll(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	t.Run("Parallel", func(t *testing.T) {
		t.Run("testCreateJobReturnsJob", testCreateJobReturnsJob)
		t.Run("TestCreateArchivedJobWithNoQueuedReturnsErrNoRows", testCreateArchivedJobWithNoQueuedReturnsErrNoRows)
		t.Run("TestArchivedJobFailsIfJobExists", testArchivedJobFailsIfJobExists)
		t.Run("TestCreateJobStoresJob", testCreateJobStoresJob)
	})
}

// Test that creating an archived job returns the job
func testCreateJobReturnsJob(t *testing.T) {
	t.Parallel()
	qj := factory.CreateQJ(t)
	ctx := context.Background()
	aj, err := archived_jobs.Create(ctx, qj.ID, qj.Name, newmodels.ArchivedJobStatusSucceeded, qj.Attempts)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, aj.ID.String(), qj.ID.String())
	test.AssertEquals(t, aj.Status, newmodels.ArchivedJobStatusSucceeded)
	test.AssertEquals(t, aj.Attempts, int16(qj.Attempts))
	test.AssertEquals(t, string(aj.Data), "{\"baz\": 17, \"foo\": [\"bar\", \"pik_345\"]}")
	test.AssertEquals(t, aj.ExpiresAt.Valid, true)
	test.Assert(t, aj.ExpiresAt.Time.Round(time.Millisecond).Equal(qj.ExpiresAt.Time.Round(time.Millisecond)), "times are equal")

	diff := time.Since(aj.CreatedAt)
	test.Assert(t, diff < 100*time.Millisecond, fmt.Sprintf("CreatedAt should be close to the current time, got %v", diff))
}

// Test that creating an archived job when the job does not exist in QueuedJobs
// returns sql.ErrNoRows
func testCreateArchivedJobWithNoQueuedReturnsErrNoRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := archived_jobs.Create(ctx, factory.JobId, "echo", newmodels.ArchivedJobStatusSucceeded, 7)
	test.AssertEquals(t, err, queued_jobs.ErrNotFound)
}

// Test that creating an archived job when one already exists returns
// a uniqueness constraint failure.
func testArchivedJobFailsIfJobExists(t *testing.T) {
	t.Parallel()
	job, qj := factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	ctx := context.Background()
	_, err := archived_jobs.Create(ctx, qj.ID, job.Name, newmodels.ArchivedJobStatusSucceeded, 7)
	test.AssertNotError(t, err, "")
	_, err = archived_jobs.Create(ctx, qj.ID, job.Name, newmodels.ArchivedJobStatusSucceeded, 7)
	test.AssertError(t, err, "expected error, got nil")
	switch terr := err.(type) {
	case *pq.Error:
		test.AssertEquals(t, terr.Code, pq.ErrorCode("23505"))
		test.AssertEquals(t, terr.Table, "archived_jobs")
		test.AssertEquals(t, terr.Message,
			`duplicate key value violates unique constraint "archived_jobs_pkey"`)
	default:
		t.Fatalf("Expected a pq.Error, got %#v", terr)
	}
}

// Test that creating a job stores the data in the database
func testCreateJobStoresJob(t *testing.T) {
	t.Parallel()
	job, qj := factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	ctx := context.Background()
	aj, err := archived_jobs.Create(ctx, qj.ID, job.Name, newmodels.ArchivedJobStatusSucceeded, 7)
	test.AssertNotError(t, err, "")
	aj, err = archived_jobs.Get(ctx, aj.ID)
	test.AssertNotError(t, err, "")

	test.AssertEquals(t, aj.ID.String(), qj.ID.String())
	test.AssertEquals(t, aj.Status, newmodels.ArchivedJobStatusSucceeded)
	test.AssertEquals(t, aj.Attempts, int16(7))
	test.AssertEquals(t, string(aj.Data), "{}")

	diff := time.Since(aj.CreatedAt)
	test.Assert(t, diff < 100*time.Millisecond, "")
}

// Test that creating an archived job when the job does not exist in QueuedJobs
// returns sql.ErrNoRows
func TestCreateArchivedJobWithWrongNameReturnsErrNoRows(t *testing.T) {
	qj := factory.CreateQueuedJob(t, factory.EmptyData)
	defer test.TearDown(t)
	ctx := context.Background()
	_, err := archived_jobs.Create(ctx, qj.ID, "wrong-job-name", newmodels.ArchivedJobStatusSucceeded, 7)
	test.AssertEquals(t, err, queued_jobs.ErrNotFound)
}
