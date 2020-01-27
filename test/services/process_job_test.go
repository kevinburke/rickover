package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	log "github.com/inconshreveable/log15"
	"github.com/kevinburke/go-types"
	"github.com/kevinburke/rest"
	"github.com/kevinburke/rickover/models/archived_jobs"
	"github.com/kevinburke/rickover/models/jobs"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/services"
	"github.com/kevinburke/rickover/test"
	"github.com/kevinburke/rickover/test/factory"
)

func TestAll(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	t.Run("Parallel", func(t *testing.T) {
		t.Run("ExpiredJobNotEnqueued", testExpiredJobNotEnqueued)
		t.Run("StatusCallbackFailedNotRetryableArchivesRecord", testStatusCallbackFailedNotRetryableArchivesRecord)
		t.Run("StatusCallbackFailedAtLeastOnceUpdatesQueuedRecord", testStatusCallbackFailedAtLeastOnceUpdatesQueuedRecord)
		t.Run("TestStatusCallbackFailedInsertsArchivedRecord", testStatusCallbackFailedInsertsArchivedRecord)
	})
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

var nullLogger = log.New()

func init() {
	nullLogger.SetHandler(log.DiscardHandler())
}

func testExpiredJobNotEnqueued(t *testing.T) {
	t.Parallel()

	c1 := make(chan bool, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		c1 <- true
	}))
	defer s.Close()
	jp := services.NewJobProcessor(services.NewDownstreamHandler(nullLogger, s.URL, "password"))

	_, err := jobs.Create(factory.SampleJob)
	test.AssertNotError(t, err, "")
	expiresAt := types.NullTime{
		Valid: true,
		Time:  time.Now().UTC().Add(-5 * time.Millisecond),
	}
	qj, err := queued_jobs.Enqueue(newParams(factory.JobId, "echo", time.Now().UTC(), expiresAt, factory.EmptyData))
	test.AssertNotError(t, err, "")
	err = jp.DoWork(context.Background(), qj)
	test.AssertNotError(t, err, "")
	for {
		select {
		case <-c1:
			t.Fatalf("worker made a request to the server")
			return
		case <-time.After(60 * time.Millisecond):
			return
		}
	}
}

// 1. Create a job type
// 2. Enqueue a job
// 3. Create a test server that replies with a 503 rest.Error
// 4. Ensure that the worker retries
func TestWorkerRetriesJSON503(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)

	// make the test go faster
	originalSleepWorker503Factor := services.UnavailableSleepFactor
	services.UnavailableSleepFactor = 0
	defer func() {
		services.UnavailableSleepFactor = originalSleepWorker503Factor
	}()

	_, err := jobs.Create(factory.SampleJob)
	test.AssertNotError(t, err, "")

	pid := types.GenerateUUID("job_")

	var data json.RawMessage
	data, err = json.Marshal(factory.RD)
	test.AssertNotError(t, err, "")
	qj, err := queued_jobs.Enqueue(newParams(pid, "echo", time.Now(), types.NullTime{Valid: false}, data))
	test.AssertNotError(t, err, "")

	var mu sync.Mutex
	count := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer func() {
			mu.Unlock()
		}()
		if count == 0 || count == 1 {
			count++
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(&rest.Error{
				Title:  "Service Unavailable",
				Detail: "The server will be shutting down momentarily and cannot accept new work.",
				ID:     "service_unavailable",
			})
		} else {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusAccepted)
			_, err = w.Write([]byte("{}"))
			test.AssertNotError(t, err, "")

			// Cheating, hit the internal success callback.
			callbackErr := services.HandleStatusCallback(context.Background(), qj.ID, "echo", newmodels.ArchivedJobStatusSucceeded, int16(5), true)
			test.AssertNotError(t, callbackErr, "")
		}
	}))
	defer s.Close()
	jp := factory.Processor(s.URL)
	err = jp.DoWork(context.Background(), qj)
	test.AssertNotError(t, err, "")
}

// this could probably be a simpler test
func TestWorkerWaitsRequestTimeout(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	var wg sync.WaitGroup
	wg.Add(1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(70 * time.Millisecond)
		wg.Done()
	}))
	defer s.Close()

	handler := services.NewDownstreamHandler(nullLogger, s.URL, "password")
	jp := services.NewJobProcessor(handler)

	qj := factory.CreateQueuedJob(t, factory.EmptyData)
	go func() {
		err := services.HandleStatusCallback(context.Background(), qj.ID, qj.Name, newmodels.ArchivedJobStatusSucceeded, qj.Attempts, true)
		test.AssertNotError(t, err, "")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	workErr := jp.DoWork(ctx, qj)
	test.AssertNotError(t, workErr, "")
	wg.Wait()
	aj, err := archived_jobs.Get(qj.ID)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, aj.Status, newmodels.ArchivedJobStatusSucceeded)
}

func TestWorkerDoesNotWaitConnectionFailure(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	handler := services.NewDownstreamHandler(
		nullLogger,
		// TODO, add empty port finder
		"http://127.0.0.1:29656",
		"password",
	)
	jp := services.NewJobProcessor(handler)

	_, qj := factory.CreateAtMostOnceJob(t, factory.EmptyData)
	err := jp.DoWork(context.Background(), qj)
	test.AssertNotError(t, err, "")
	aj, err := archived_jobs.Get(qj.ID)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, aj.Status, newmodels.ArchivedJobStatusFailed)
}
