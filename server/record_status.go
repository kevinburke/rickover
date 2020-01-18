package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	metrics "github.com/kevinburke/go-simple-metrics"
	"github.com/kevinburke/rest"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/services"
)

// jobStatusUpdater satisfies the Handler interface.
type jobStatusUpdater struct{}

// The body of a POST request to /v1/jobs/:job-name/:job-id, recording the
// status of a job.
type JobStatusRequest struct {
	// Should be "succeeded" or "failed".
	Status newmodels.ArchivedJobStatus `json:"status"`

	// Attempt is sent to ensure we don't attempt a null write.
	Attempt *int16 `json:"attempt"` // pointer to distinguish between null/omitted value and 0.

	// Retryable indicates whether a failure is retryable. The default is true.
	// Set to false to avoid retrying a particular failure.
	Retryable *bool `json:"retryable"` // pointer to distinguish between null value and false.
}

// POST /v1/jobs/:name/:id
//
// Update a job's status with success or failure
func (j *jobStatusUpdater) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		badRequest(w, r, createEmptyErr("status", r.URL.Path))
		return
	}
	defer r.Body.Close()
	var jsr JobStatusRequest
	err := json.NewDecoder(r.Body).Decode(&jsr)
	if err != nil {
		badRequest(w, r, &rest.Error{
			ID:    "invalid_request",
			Title: "Invalid request: bad JSON. Double check the types of the fields you sent",
		})
		return
	}
	if jsr.Status == "" {
		badRequest(w, r, createEmptyErr("status", r.URL.Path))
		return
	}
	if jsr.Attempt == nil {
		badRequest(w, r, createEmptyErr("attempt", r.URL.Path))
		return
	}
	if jsr.Status != newmodels.ArchivedJobStatusSucceeded && jsr.Status != newmodels.ArchivedJobStatusFailed {
		badRequest(w, r, &rest.Error{
			ID:       "invalid_status",
			Title:    fmt.Sprintf("Invalid job status: %s", jsr.Status),
			Instance: r.URL.Path,
		})
		return
	}
	name := jobIdRoute.FindStringSubmatch(r.URL.Path)[1]
	idStr := jobIdRoute.FindStringSubmatch(r.URL.Path)[2]
	id, done := getId(w, r, idStr)
	if done {
		return
	}
	if jsr.Retryable == nil {
		// http://stackoverflow.com/q/30716354/329700
		jsr.Retryable = func() *bool { b := true; return &b }()
	}
	err = services.HandleStatusCallback(context.TODO(), id, name, jsr.Status, *jsr.Attempt, *jsr.Retryable)
	if err == nil {
		w.WriteHeader(http.StatusOK)
	} else if err == queued_jobs.ErrNotFound {
		badRequest(w, r, &rest.Error{
			ID:       "duplicate_status_request",
			Title:    "This job has already been archived, or was never queued",
			Instance: r.URL.Path,
		})
		metrics.Increment("status_callback.duplicate")
		return
	} else {
		writeServerError(w, r, err)
		metrics.Increment("status_callback.error")
	}
}
