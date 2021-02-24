package servertest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kevinburke/go-types"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/server"
	"github.com/kevinburke/rickover/test"
	"github.com/kevinburke/rickover/test/factory"
)

func TestV2(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	t.Run("CreateJobReturnsJob", func(t *testing.T) {
		w := httptest.NewRecorder()
		b := new(bytes.Buffer)
		json.NewEncoder(b).Encode(validRequest)
		req, err := http.NewRequest("POST", "/v2/job-types", b)
		test.AssertNotError(t, err, "")
		req.SetBasicAuth("foo", "bar")
		server.Get(u).ServeHTTP(w, req)
		test.AssertEquals(t, w.Code, http.StatusCreated)
		job := new(newmodels.Job)
		err = json.NewDecoder(w.Body).Decode(job)
		test.AssertNotError(t, err, "")
		test.AssertEquals(t, job.Name, validRequest.Name)
		test.AssertEquals(t, job.Attempts, validRequest.Attempts)
		test.AssertEquals(t, job.Concurrency, validRequest.Concurrency)
		test.AssertEquals(t, job.DeliveryStrategy, validRequest.DeliveryStrategy)
		diff := time.Since(job.CreatedAt)
		test.Assert(t, diff < 25*time.Millisecond, fmt.Sprintf("diff: %v created: %v", diff, job.CreatedAt))
	})

	t.Run("RetrieveCreatedJob", func(t *testing.T) {
		job := factory.CreateJob(t, newmodels.CreateJobParams{
			Name: types.GenerateUUID("").String()[:8],
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v2/job-types/"+job.Name, nil)
		req.SetBasicAuth("foo", "bar")
		server.Get(u).ServeHTTP(w, req)
		test.AssertEquals(t, w.Code, http.StatusOK)
		var qj newmodels.Job
		err := json.NewDecoder(w.Body).Decode(&qj)
		test.AssertNotError(t, err, "")
		test.AssertEquals(t, qj.Name, job.Name)
	})
}
