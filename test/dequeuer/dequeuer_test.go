package dequeuer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/kevinburke/rickover/dequeuer"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/test"
	"github.com/kevinburke/rickover/test/factory"
	"github.com/kevinburke/semaphore"
)

func TestAll(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	t.Run("Parallel", func(t *testing.T) {
		t.Run("TestWorkerShutsDown", testWorkerShutsDown)
		t.Run("TestWorkerShutsDownWithContext", testWorkerShutsDownWithContext)
		t.Run("TestWorkerMakesCorrectRequest", testWorkerMakesCorrectRequest)
		t.Run("TestWorkerMakesExactlyOneRequest", testWorkerMakesExactlyOneRequest)
	})
}

func testWorkerShutsDown(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	poolname := factory.RandomId("pool")
	pool := dequeuer.NewPool(ctx, poolname.String())
	for i := 0; i < 3; i++ {
		pool.AddDequeuer(ctx, factory.Processor("http://example.com"))
	}
	c1 := make(chan struct{}, 1)
	go func() {
		err := pool.Shutdown(ctx)
		test.AssertNotError(t, err, "")
		c1 <- struct{}{}
	}()
	for {
		select {
		case <-c1:
			return
		case <-time.After(300 * time.Millisecond):
			t.Fatalf("pool did not shut down in 300ms")
		}
	}
}

func testWorkerShutsDownWithContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	poolname := factory.RandomId("pool")
	pool := dequeuer.NewPool(ctx, poolname.String())
	for i := 0; i < 3; i++ {
		pool.AddDequeuer(ctx, factory.Processor("http://example.com"))
	}
	c1 := make(chan struct{}, 1)
	go func() {
		for {
			if pool.Len() == 0 {
				c1 <- struct{}{}
				break
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	cancel()
	for {
		select {
		case <-c1:
			return
		case <-time.After(300 * time.Millisecond):
			t.Fatalf("pool did not shut down in 300ms")
		}
	}
}

// 1. Create a job type
// 2. Enqueue a job
// 3. Create a test server that replies with a 202
// 4. Ensure that the correct request is made to the server
func testWorkerMakesCorrectRequest(t *testing.T) {
	t.Parallel()
	qj := factory.CreateQJ(t)

	c1 := make(chan bool, 1)
	var path, method, user string
	var ok bool
	var workRequest struct {
		Data     *factory.RandomData `json:"data"`
		Attempts int16               `json:"attempts"`
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		method = r.Method
		user, _, ok = r.BasicAuth()
		err := json.NewDecoder(r.Body).Decode(&workRequest)
		test.AssertNotError(t, err, "decoding request body")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
		c1 <- true
		close(c1)
	}))
	defer s.Close()
	jp := factory.Processor(s.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := dequeuer.NewPool(ctx, qj.Name)
	pool.AddDequeuer(ctx, jp)
	defer pool.Shutdown(context.Background())
	select {
	case <-c1:
		test.AssertEquals(t, path, fmt.Sprintf("/v1/jobs/%s/%s", qj.Name, qj.ID.String()))
		test.AssertEquals(t, method, "POST")
		test.AssertEquals(t, ok, true)
		test.AssertEquals(t, user, "jobs")
		test.AssertDeepEquals(t, workRequest.Data, factory.RD)
		test.AssertEquals(t, workRequest.Attempts, qj.Attempts)
		return
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("Server did not receive a request in 200ms, quitting")
	}
}

// 1. Create a job type
// 2. Enqueue a job
// 2a. Create twenty worker nodes
// 3. Create a test server that replies with a 202
// 4. Ensure that only one request is made to the server
func testWorkerMakesExactlyOneRequest(t *testing.T) {
	t.Parallel()
	qj := factory.CreateQJ(t)

	c1 := make(chan bool, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
		c1 <- true
	}))
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	pool := dequeuer.NewPool(ctx, qj.Name)
	for i := 0; i < 20; i++ {
		jp := factory.Processor(s.URL)
		pool.AddDequeuer(ctx, jp)
	}
	defer cancel()
	count := 0
	for {
		select {
		case <-c1:
			count++
		case <-time.After(100 * time.Millisecond):
			test.AssertEquals(t, count, 1)
			return
		}
	}
}

func TestCreatePools(t *testing.T) {
	test.SetUp(t)
	defer test.TearDown(t)
	qj := factory.CreateQJ(t)
	factory.CreateQJ(t)
	proc := factory.Processor("http://example.com")
	ctx := context.Background()
	pools, err := dequeuer.CreatePools(ctx, proc, 0)
	test.AssertNotError(t, err, "CreatePools")
	test.AssertEquals(t, len(pools), 2)
	foundPool := false
	for _, pool := range pools {
		if pool.Name == qj.Name {
			foundPool = true
			test.AssertEquals(t, len(pool.Dequeuers), 3)
			for i, dq := range pool.Dequeuers {
				test.AssertEquals(t, dq.ID, i+1)
				test.AssertEquals(t, dq.W, proc)
			}
		}
	}
	test.Assert(t, foundPool, "Didn't create a pool for the job type")
}

func runDQBench(b *testing.B, populate bool, concurrency int16) {
	buf := new(bytes.Buffer)
	log.SetOutput(buf)
	defer func() {
		if b.Failed() {
			io.Copy(os.Stdout, buf)
		}
		log.SetOutput(os.Stdout)
	}()
	test.SetUp(b)
	// defer test.TearDown(b)
	data, _ := json.Marshal(factory.RD)
	if populate {
		if _, err := newmodels.DB.DeleteAllQueuedJobs(context.Background()); err != nil {
			b.Fatal(err)
		}
		job := factory.CreateJob(b, newmodels.CreateJobParams{
			Name:             factory.RandomId("").String()[:8],
			Concurrency:      concurrency,
			DeliveryStrategy: newmodels.DeliveryStrategyAtLeastOnce,
			Attempts:         1,
		})
		var wg sync.WaitGroup
		sem := semaphore.New(16)
		// Idea here is basically to insert enough jobs that the dequeuer always has
		// work to do, no matter how long the benchmark takes to run.
		for j := 0; j < 50000; j++ {
			sem.Acquire()
			wg.Add(1)
			go func() {
				defer sem.Release()
				defer wg.Done()
				factory.CreateQueuedJobOnly(b, job.Name, data)
			}()
		}
		wg.Wait()
	}
	w := &ChannelProcessor{
		Ch: make(chan struct{}, 1000),
	}
	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	ctx := context.Background()
	pools, err := dequeuer.CreatePools(ctx, w, 0)
	test.AssertNotError(b, err, "CreatePools")
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		for _, pool := range pools {
			pool.Shutdown(ctx)
		}
		cancel()
	}()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			<-w.Ch
		}
	})
	count, err := newmodels.DB.CountReadyAndAll(context.Background())
	if err != nil {
		b.Fatal(err)
	}
	if count.All == 0 {
		b.Fatal("benchmark removed all rows from the queued_jobs table")
	}
}

func BenchmarkDequeue(b *testing.B) {
	b.Run("Dequeue1", func(b1 *testing.B) { runDQBench(b1, false, 1) })
	b.Run("Dequeue4", func(b4 *testing.B) { runDQBench(b4, false, 4) })
	b.Run("Dequeue8", func(b8 *testing.B) { runDQBench(b8, false, 8) })
	b.Run("Dequeue16", func(b16 *testing.B) { runDQBench(b16, false, 16) })
	b.Run("Dequeue64", func(b64 *testing.B) { runDQBench(b64, false, 64) })
	b.Run("Dequeue128", func(b128 *testing.B) { runDQBench(b128, false, 128) })
}
