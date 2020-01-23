package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

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
	// Amount of time we should wait for the Handler to perform the work before
	// marking the job as failed.
	Timeout time.Duration

	Handler Handler
}

type Handler interface {
	// Handle is responsible for notifying some downstream thing that there is
	// work to be done.
	Handle(context.Context, *newmodels.QueuedJob) error
}

type DownstreamHandler struct {
	// A Client for making requests to the downstream server.
	client *downstream.Client
}

func (d *DownstreamHandler) Handle(ctx context.Context, qj *newmodels.QueuedJob) error {
	log.Printf("processing job %s (type %s)", qj.ID.String(), qj.Name)
	for i := uint8(0); i < 3; i++ {
		if qj.ExpiresAt.Valid && time.Since(qj.ExpiresAt.Time) >= 0 {
			return createAndDelete(ctx, qj.ID, qj.Name, newmodels.ArchivedJobStatusExpired, qj.Attempts)
		}
		params := &downstream.JobParams{
			Data:     qj.Data,
			Attempts: qj.Attempts,
		}
		start := time.Now()
		err := d.client.Job.Post(ctx, qj.Name, &qj.ID, params)
		go metrics.Time("post_job.latency", time.Since(start))
		go metrics.Time(fmt.Sprintf("post_job.%s.latency", qj.Name), time.Since(start))
		if err == nil {
			go metrics.Increment(fmt.Sprintf("post_job.%s.accepted", qj.Name))
			return nil
		} else {
			switch aerr := err.(type) {
			case *rest.Error:
				if aerr.ID == "service_unavailable" {
					go metrics.Increment("post_job.unavailable")
					time.Sleep(time.Duration(1<<i*UnavailableSleepFactor) * time.Millisecond)
					continue
				}
				go metrics.Increment("dequeue.post_job.error")
				return err
			default:
				go func(err error) {
					if reachedRemoteServer(err) {
						metrics.Increment("dequeue.post_job.timeout")
					} else {
						log.Printf("Unknown error making POST request to downstream server: %q (%#v)", err, err)
						metrics.Increment("dequeue.post_job.error_unknown")
					}
				}(err)
				return err
			}
		}
	}
	return nil
}

func NewDownstreamHandler(downstreamUrl string, downstreamPassword string) *DownstreamHandler {
	return &DownstreamHandler{
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
		Timeout: DefaultTimeout,
		Handler: h,
	}
}

// reachedRemoteServer returns true if the err may have occurred after the
// remote server was contacted.
func reachedRemoteServer(err error) bool {
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		fmt.Printf("net err: %#v\n", netErr)
		if netErr.Op == "dial" {
			return false
		}
	}
	var sysErr *os.SyscallError
	if errors.As(err, &sysErr) {
		fmt.Printf("sys err: %#v\n", sysErr)
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
			fmt.Println("wait for job", err)
			return waitForJob(ctx, qj, jp.Timeout)
		} else {
			// Assume the request failed.
			fmt.Println("handle status callback")
			return HandleStatusCallback(ctx, qj.ID, qj.Name, newmodels.ArchivedJobStatusFailed, qj.Attempts, true)
		}
	}
	return waitForJob(ctx, qj, jp.Timeout)
}

// waitForJob waits for another thread to update the status of the job in
// the queued_jobs table.
func waitForJob(ctx context.Context, qj *newmodels.QueuedJob, failTimeout time.Duration) error {
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
			go metrics.Increment(fmt.Sprintf("wait_for_job.%s.timeout", name))
			log.Printf("5 minutes elapsed, marking %s (type %s) as failed", idStr, name)
			err := HandleStatusCallback(ctx, qj.ID, name, newmodels.ArchivedJobStatusFailed, currentAttemptCount, true)
			go metrics.Increment(fmt.Sprintf("wait_for_job.%s.failed", name))
			log.Printf("job %s (type %s) timed out after waiting for %v", idStr, name, time.Since(start))
			if err == sql.ErrNoRows {
				// Attempted to decrement the failed count, but couldn't do so;
				// we assume another thread got here before we did.
				return nil
			}
			if err != nil {
				log.Printf("error marking job %s as failed: %s\n", idStr, err.Error())
				go metrics.Increment(fmt.Sprintf("wait_for_job.%s.failed.error", name))
			}
			return err
		default:
			getStart := time.Now()
			qj, err := queued_jobs.Get(tctx, qj.ID)
			queryCount++
			go metrics.Time("wait_for_job.get.latency", time.Since(getStart))
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
					log.Printf("job %s (type %s) completed after %s", idStr, name, roundDuration)
				}(name, start, idStr, queryCount)
				return nil
			} else if err != nil {
				continue
			}
			if qj.Attempts < currentAttemptCount {
				// Another thread decremented the attempt count and re-queued
				// the job, we're done.
				go metrics.Time(fmt.Sprintf("wait_for_job.%s.latency", name), time.Since(start))
				go metrics.Increment(fmt.Sprintf("wait_for_job.%s.attempt_count_decremented", name))
				log.Printf("job %s (type %s) failed after %v, retrying", idStr, name, time.Since(start))
				return nil
			}
		}
	}
}
