package test_jobs

import (
	"fmt"
	"testing"
	"time"

	types "github.com/kevinburke/go-types"
	"github.com/kevinburke/rickover/models/jobs"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/test"
)

func TestAll(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	t.Run("Parallel", func(t *testing.T) {
		t.Run("CreateMissingFields", testCreateMissingFields)
		t.Run("CreateInvalidFields", testCreateInvalidFields)
		t.Run("CreateReturnsRecord", testCreateReturnsRecord)
		t.Run("Get", testGet)
	})
}

func testCreateMissingFields(t *testing.T) {
	t.Parallel()
	job := newmodels.CreateJobParams{
		Name: "email-signup",
	}
	_, err := jobs.Create(job)
	test.AssertError(t, err, "")
	test.AssertEquals(t, err.Error(), "Invalid delivery_strategy: \"\"")
}

func testCreateInvalidFields(t *testing.T) {
	t.Parallel()
	test.SetUp(t)
	job := newmodels.CreateJobParams{
		Name:             "email-signup",
		DeliveryStrategy: newmodels.DeliveryStrategy("foo"),
	}
	_, err := jobs.Create(job)
	test.AssertError(t, err, "")
	test.AssertEquals(t, err.Error(), "Invalid delivery_strategy: \"foo\"")
}

func newJob(t *testing.T) newmodels.CreateJobParams {
	t.Helper()
	id := types.GenerateUUID("jobname_")
	return newmodels.CreateJobParams{
		Name:             id.String(),
		DeliveryStrategy: newmodels.DeliveryStrategyAtLeastOnce,
		Attempts:         3,
		Concurrency:      1,
	}
}

func testCreateReturnsRecord(t *testing.T) {
	t.Parallel()
	j0 := newJob(t)
	j, err := jobs.Create(j0)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, j.Name, j0.Name)
	test.AssertEquals(t, j.DeliveryStrategy, newmodels.DeliveryStrategyAtLeastOnce)
	test.AssertEquals(t, j.Attempts, int16(3))
	test.AssertEquals(t, j.Concurrency, int16(1))
	diff := time.Since(j.CreatedAt)
	test.Assert(t, diff < 100*time.Millisecond, fmt.Sprintf("CreatedAt should be close to the current time, got %v", diff))
}

func testGet(t *testing.T) {
	t.Parallel()
	j0 := newJob(t)
	_, err := jobs.Create(j0)
	test.AssertNotError(t, err, "")
	j, err := jobs.Get(j0.Name)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, j.Name, j0.Name)
	test.AssertEquals(t, j.DeliveryStrategy, newmodels.DeliveryStrategyAtLeastOnce)
	test.AssertEquals(t, j.Attempts, int16(3))
	test.AssertEquals(t, j.Concurrency, int16(1))
	diff := time.Since(j.CreatedAt)
	test.Assert(t, diff < 100*time.Millisecond, "")
}
