package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/kevinburke/go-types"
	"github.com/kevinburke/rest"
	"github.com/kevinburke/rest/resterror"
	"github.com/kevinburke/rickover/metrics"
	"github.com/kevinburke/rickover/models/archived_jobs"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/services"
)

// POST /v1/jobs(/:name)/:id/replay
//
// Replay a given job. Generates a new UUID and then enqueues the job based on
// the original.
func replayHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// first capturing group is /:name, 2nd is the name
		name := replayRoute.FindStringSubmatch(r.URL.Path)[2]
		idStr := replayRoute.FindStringSubmatch(r.URL.Path)[3]
		id, done := getId(w, r, idStr)
		if done {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var jobName string
		var data json.RawMessage
		qj, err := queued_jobs.GetRetry(ctx, id, 3)
		var expiresAt types.NullTime
		if err == nil {
			if qj.Status == newmodels.JobStatusQueued {
				apierr := &resterror.Error{
					Title:    "Cannot replay a queued job. Wait for it to start",
					ID:       "invalid_replay_attempt",
					Instance: r.URL.Path,
				}
				rest.BadRequest(w, r, apierr)
				return
			}
			jobName = qj.Name
			data = qj.Data
			expiresAt = qj.ExpiresAt
		} else if err == queued_jobs.ErrNotFound {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			aj, err := archived_jobs.GetRetry(ctx, id, 3)
			cancel()
			if err == nil {
				jobName = aj.Name
				data = aj.Data
				expiresAt = aj.ExpiresAt
			} else if err == archived_jobs.ErrNotFound {
				notFound(w, new404(r))
				metrics.Increment("job.replay.not_found")
				return
			} else {
				rest.ServerError(w, r, err)
				metrics.Increment("job.replay.get.error")
				return
			}
		} else {
			rest.ServerError(w, r, err)
			metrics.Increment("job.replay.get.error")
			return
		}

		if name != "" && jobName != name {
			nfe := &resterror.Error{
				Title:    "Job exists, but with a different name",
				ID:       "job_not_found",
				Instance: r.URL.Path,
			}
			notFound(w, nfe)
			return
		}

		newId := types.GenerateUUID("job_")
		params := newmodels.EnqueueJobParams{
			ID: newId, Name: jobName, RunAfter: time.Now(),
			ExpiresAt: expiresAt, Data: data,
		}
		queuedJob, err := services.Enqueue(ctx, newmodels.DB,
			params)
		if err != nil {
			rest.ServerError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(queuedJob)
		metrics.Increment("enqueue.replay.success")
	})
}
