package services

import (
	"context"
	"testing"

	"github.com/kevinburke/rickover/models/archived_jobs"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/services"
	"github.com/kevinburke/rickover/test"
	"github.com/kevinburke/rickover/test/factory"
)

func TestStatusCallbackInsertsArchivedRecordDeletesQueuedRecord(t *testing.T) {
	defer test.TearDown(t)
	qj := factory.CreateQueuedJob(t, factory.EmptyData)
	err := services.HandleStatusCallback(context.Background(), nullLogger, qj.ID, "echo", newmodels.ArchivedJobStatusSucceeded, 7, true)
	test.AssertNotError(t, err, "")
	_, err = queued_jobs.Get(context.Background(), qj.ID)
	test.AssertEquals(t, err, queued_jobs.ErrNotFound)
	aj, err := archived_jobs.Get(qj.ID)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, aj.ID.String(), qj.ID.String())
	test.AssertEquals(t, aj.Attempts, int16(7))
	test.AssertEquals(t, aj.Name, "echo")
	test.AssertEquals(t, aj.Status, newmodels.ArchivedJobStatusSucceeded)
}

func testStatusCallbackFailedInsertsArchivedRecord(t *testing.T) {
	t.Parallel()
	job, qj := factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	err := services.HandleStatusCallback(context.Background(), nullLogger, qj.ID, job.Name, newmodels.ArchivedJobStatusFailed, 1, true)
	test.AssertNotError(t, err, "")
	_, err = queued_jobs.Get(context.Background(), qj.ID)
	test.AssertEquals(t, err, queued_jobs.ErrNotFound)
	aj, err := archived_jobs.Get(qj.ID)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, aj.ID.String(), qj.ID.String())
}

func TestStatusCallbackFailedAtMostOnceInsertsArchivedRecord(t *testing.T) {
	defer test.TearDown(t)
	_, qj := factory.CreateAtMostOnceJob(t, factory.EmptyData)
	err := services.HandleStatusCallback(context.Background(), nullLogger, qj.ID, "at-most-once", newmodels.ArchivedJobStatusFailed, 7, true)
	test.AssertNotError(t, err, "")
	_, err = queued_jobs.Get(context.Background(), qj.ID)
	test.AssertEquals(t, err, queued_jobs.ErrNotFound)
	aj, err := archived_jobs.Get(qj.ID)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, aj.ID.String(), qj.ID.String())
}

func testStatusCallbackFailedAtLeastOnceUpdatesQueuedRecord(t *testing.T) {
	t.Parallel()
	job, qj := factory.CreateUniqueQueuedJob(t, factory.EmptyData)
	err := services.HandleStatusCallback(context.Background(), nullLogger, qj.ID, job.Name, newmodels.ArchivedJobStatusFailed, 7, true)
	test.AssertNotError(t, err, "")

	qj, err = queued_jobs.Get(context.Background(), qj.ID)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, qj.Attempts, int16(6))

	_, err = archived_jobs.Get(qj.ID)
	test.AssertEquals(t, err, archived_jobs.ErrNotFound)
}

func testStatusCallbackFailedNotRetryableArchivesRecord(t *testing.T) {
	t.Parallel()
	qj := factory.CreateQJ(t)
	err := services.HandleStatusCallback(context.Background(), nullLogger, qj.ID, qj.Name, newmodels.ArchivedJobStatusFailed, qj.Attempts, false)
	test.AssertNotError(t, err, "inserting archived record")

	_, err = queued_jobs.Get(context.Background(), qj.ID)
	test.AssertEquals(t, err, queued_jobs.ErrNotFound)
	aj, err := archived_jobs.Get(qj.ID)
	test.AssertNotError(t, err, "finding archived job")
	test.AssertEquals(t, aj.Status, newmodels.ArchivedJobStatusFailed)
	test.AssertEquals(t, aj.Attempts, qj.Attempts-1)
}

// This test returns an error - if the queued job doesn't exist, we can't
// create an archived job.
func TestStatusCallbackFailedAtMostOnceArchivedRecordExists(t *testing.T) {
	defer test.TearDown(t)
	aj := factory.CreateArchivedJob(t, factory.EmptyData, newmodels.ArchivedJobStatusFailed)
	err := services.HandleStatusCallback(context.Background(), nullLogger, aj.ID, aj.Name, newmodels.ArchivedJobStatusFailed, 1, true)
	test.AssertEquals(t, err, queued_jobs.ErrNotFound)
}
