package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	log "github.com/inconshreveable/log15"
	metrics "github.com/kevinburke/go-simple-metrics"
	"github.com/kevinburke/rest"
	"github.com/kevinburke/rickover/downstream"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
)

// UnavailableSleepFactor determines how long the application should sleep
// between 503 Service Unavailable downstream responses.
var UnavailableSleepFactor = 500

// DefaultTimeout is the default amount of time a JobProcessor should wait for
// a job to complete, once it's been sent to the downstream server.
var DefaultTimeout = 5 * time.Minute

// JobProcessor is the default implementation of the Worker interface.
type JobProcessor struct {
	log.Logger
	// Amount of time we should wait for the Handler to perform the work before
	// marking the job as failed.
	Timeout time.Duration

	Handler Handler
}

type Handler interface {
	log.Logger
	// Handle is responsible for notifying some downstream thing that there is
	// work to be done.
	Handle(context.Context, *newmodels.QueuedJob) error
}

type DownstreamHandler struct {
	log.Logger
	// A Client for making requests to the downstream server.
	client *downstream.Client
}

func (d *DownstreamHandler) Handle(ctx context.Context, qj *newmodels.QueuedJob) error {
	d.Info("processing job", "id", qj.ID.String(), "type", qj.Name)
	for i := 0; i < 3; i++ {
		params := &downstream.JobParams{
			Data:     qj.Data,
			Attempts: qj.Attempts,
		}
		start := time.Now()
		err := d.client.Job.Post(ctx, qj.Name, &qj.ID, params)
		metrics.Time("post_job.latency", time.Since(start))
		metrics.Time(fmt.Sprintf("post_job.%s.latency", qj.Name), time.Since(start))
		if err == nil {
			metrics.Increment(fmt.Sprintf("post_job.%s.accepted", qj.Name))
			return nil
		} else {
			switch aerr := err.(type) {
			case *rest.Error:
				if aerr.ID == "service_unavailable" {
					metrics.Increment("post_job.unavailable")
					time.Sleep(time.Duration(1<<i*UnavailableSleepFactor) * time.Millisecond)
					continue
				}
				metrics.Increment("dequeue.post_job.error")
				return err
			default:
				go func(err error) {
					if reachedRemoteServer(err) {
						metrics.Increment("dequeue.post_job.timeout")
					} else {
						d.Error("error making POST request to downstream server", "err", err)
						metrics.Increment("dequeue.post_job.error_unknown")
					}
				}(err)
				return err
			}
		}
	}
	return nil
}

func NewDownstreamHandler(logger log.Logger, downstreamUrl string, downstreamPassword string) *DownstreamHandler {
	return &DownstreamHandler{
		Logger: logger,
		client: downstream.NewClient("jobs", downstreamPassword, downstreamUrl),
	}
}

// NewJobProcessor creates a services.JobProcessor that makes requests to the
// downstream url.
//
// By default the Client uses Basic Auth with "jobs" as the username, and the
// configured password as the password.
//
// If the downstream server does not hit the callback, jobs sent to the
// downstream server are timed out and marked as failed after DefaultTimeout
// has elapsed.
func NewJobProcessor(h Handler) *JobProcessor {
	return &JobProcessor{
		Logger:  h,
		Timeout: DefaultTimeout,
		Handler: h,
	}
}

// reachedRemoteServer returns true if the err may have occurred after the
// remote server was contacted.
func reachedRemoteServer(err error) bool {
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if netErr.Op == "dial" {
			return false
		}
	}
	var sysErr *os.SyscallError
	if errors.As(err, &sysErr) {
		if sysErr.Syscall == "connect" {
			return false
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		// We can't really introspect the phase of the HTTP process, just
		// assume these are all timeouts.
		return true
	}
	return strings.Contains(err.Error(), "Timeout exceeded") || strings.Contains(err.Error(), "i/o timeout")
}

// DoWork sends the given queued job to the downstream service, then waits for
// it to complete.
func (jp *JobProcessor) DoWork(ctx context.Context, qj *newmodels.QueuedJob) error {
	if jp == nil || jp.Handler == nil {
		panic("cannot do work with nil Handler")
	}
	if qj.ExpiresAt.Valid && time.Since(qj.ExpiresAt.Time) >= 0 {
		return createAndDelete(ctx, qj.ID, qj.Name, newmodels.ArchivedJobStatusExpired, qj.Attempts)
	}
	var tctx context.Context
	var cancel context.CancelFunc
	deadline, ok := ctx.Deadline()
	if ok && time.Until(deadline) > 30*time.Millisecond {
		// reserve 30ms for the database after deadline; it's not great but better than nothing
		tctx, cancel = context.WithDeadline(ctx, deadline.Add(-30*time.Millisecond))
	} else {
		tctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()
	if err := jp.Handler.Handle(tctx, qj); err != nil {
		if reachedRemoteServer(err) {
			// Special case: if the request made it to the downstream server,
			// but we timed out waiting for a response, we assume the downstream
			// server received it and will handle it. we see this most often
			// when the downstream server restarts. Heroku receives/queues the
			// requests until the new server is ready, and we see a timeout.
			return waitForJob(ctx, jp.Logger, qj, jp.Timeout)
		} else {
			// Assume the request failed.
			return HandleStatusCallback(ctx, qj.ID, qj.Name, newmodels.ArchivedJobStatusFailed, qj.Attempts, true)
		}
	}
	return waitForJob(ctx, jp.Logger, qj, jp.Timeout)
}

// waitForJob waits for another thread to update the status of the job in
// the queued_jobs table.
func waitForJob(ctx context.Context, logger log.Logger, qj *newmodels.QueuedJob, failTimeout time.Duration) error {
	start := time.Now()
	// This is not going to change but we continually overwrite qj
	name := qj.Name
	idStr := qj.ID.String()

	currentAttemptCount := qj.Attempts
	queryCount := int64(0)
	if failTimeout <= 0 {
		failTimeout = DefaultTimeout
	}
	tctx, cancel := context.WithTimeout(ctx, failTimeout)
	defer cancel()
	for {
		select {
		case <-tctx.Done():
			metrics.Increment(fmt.Sprintf("wait_for_job.%s.timeout", name))
			logger.Info("timeout exceeded, marking job as failed", "id", idStr, "type", name)
			err := HandleStatusCallback(ctx, qj.ID, name, newmodels.ArchivedJobStatusFailed, currentAttemptCount, true)
			metrics.Increment(fmt.Sprintf("wait_for_job.%s.failed", name))
			if err == sql.ErrNoRows {
				// Attempted to decrement the failed count, but couldn't do so;
				// we assume another thread got here before we did.
				return nil
			}
			if err != nil {
				logger.Error("error marking job as failed", "id", idStr, "err", err)
				metrics.Increment(fmt.Sprintf("wait_for_job.%s.failed.error", name))
			}
			return err
		default:
			getStart := time.Now()
			qj, err := queued_jobs.Get(tctx, qj.ID)
			queryCount++
			metrics.Time("wait_for_job.get.latency", time.Since(getStart))
			if err == queued_jobs.ErrNotFound {
				// inserted this job into archived_jobs. nothing to do!
				go func(name string, start time.Time, idStr string, queryCount int64) {
					metrics.Increment(fmt.Sprintf("wait_for_job.%s.archived", name))
					metrics.Increment("wait_for_job.archived")
					metrics.Time(fmt.Sprintf("wait_for_job.%s.latency", name), time.Since(start))
					metrics.Measure(fmt.Sprintf("wait_for_job.%s.queries", name), queryCount)
					duration := time.Since(start)
					// Default print method has too many decimals
					roundDuration := duration - duration%(time.Millisecond/10)
					logger.Info("job completed", "id", idStr, "name", name, "duration", roundDuration)
				}(name, start, idStr, queryCount)
				return nil
			} else if err != nil {
				continue
			}
			if qj.Attempts < currentAttemptCount {
				// Another thread decremented the attempt count and re-queued
				// the job, we're done.
				metrics.Time(fmt.Sprintf("wait_for_job.%s.latency", name), time.Since(start))
				metrics.Increment(fmt.Sprintf("wait_for_job.%s.attempt_count_decremented", name))
				logger.Info("job failed, retrying", "id", idStr, "type", name, "duration", time.Since(start))
				return nil
			}
		}
	}
}

func (jp *JobProcessor) Sleep(failedAttempts int32) time.Duration {
	if failedAttempts <= 2 {
		return 0
	}
	// this is not very scientific at all.
	switch failedAttempts {
	case 3, 4:
		return 50 * time.Millisecond
	case 5:
		return 100 * time.Millisecond
	case 6:
		return 200 * time.Millisecond
	case 7:
		return 400 * time.Millisecond
	default:
		return time.Second
	}
}
