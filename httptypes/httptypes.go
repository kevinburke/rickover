// Package httptypes contains types that are serialized in HTTP request bodies
// or responses.
package httptypes

import (
	"encoding/json"
	"time"

	types "github.com/kevinburke/go-types"
)

type Job struct {
	Name             string    `json:"name"`
	DeliveryStrategy string    `json:"delivery_strategy"`
	Attempts         int16     `json:"attempts"`
	Concurrency      int16     `json:"concurrency"`
	CreatedAt        time.Time `json:"created_at"`
}

// An EnqueueJobRequest is sent in the body of a request to PUT
// /v1/jobs/:job-name/:job-id.
type EnqueueJobRequest struct {
	// Job data to enqueue.
	Data json.RawMessage `json:"data"`
	// The earliest time we can run this job. If not specified, defaults to the
	// current time.
	RunAfter types.NullTime `json:"run_after"`
	// The latest time we can run this job. If not specified, defaults to null
	// (never expires).
	ExpiresAt types.NullTime `json:"expires_at"`
}

// The body of a POST request to /v1/jobs/:job-name/:job-id, recording the
// status of a job.
type JobStatusRequest struct {
	// Should match one of the values in newmodels.ArchivedJobStatus
	// ("succeeded" or "failed").
	Status string `json:"status"`

	// Attempt is sent to ensure we don't attempt a null write.
	Attempt *int16 `json:"attempt"` // pointer to distinguish between null/omitted value and 0.

	// Retryable indicates whether a failure is retryable. The default is true.
	// Set to false to avoid retrying a particular failure.
	Retryable *bool `json:"retryable"` // pointer to distinguish between null value and false.
}

// CreateJobTypeRequest contains data sent in the body of a request to create
// a job type.
type CreateJobTypeRequest struct {
	Name        string `json:"name"`
	Attempts    int16  `json:"attempts"`
	Concurrency int16  `json:"concurrency"`
	// Should match the values in the newmodels.DeliveryStrategy enum
	// - "at_most_once" or "at_least_once"
	DeliveryStrategy string `json:"delivery_strategy"`
}
