// Package dequeuer retrieves jobs from the database and does some work.
package dequeuer

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/inconshreveable/log15"
	"github.com/kevinburke/rickover/metrics"
	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/models/jobs"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/services"
	"github.com/kevinburke/rickover/setup"
	"golang.org/x/sync/errgroup"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type WorkServer struct {
	log.Logger
	processor           *services.JobProcessor
	stuckJobTimeout     time.Duration
	disableMetaShutdown bool
}

type Config struct {
	// Database connector, for example db.DatabaseURLConnector. If nil,
	// db.DefaultConnection is used.
	Connector db.Connector
	// Number of open connections to the database
	NumConns        int
	Processor       *services.JobProcessor
	StuckJobTimeout time.Duration

	// Enqueueing a job with name "meta.shutdown" will shutdown the dequeuer (so
	// it can be restarted with a job type added or removed).
	//
	// Enable this flag if you have long running jobs that could be interfered
	// with if the dequeuer restarted.
	DisableMetaShutdown bool

	Logger log.Logger
}

// New creates a new WorkServer.
func New(ctx context.Context, cfg Config) (WorkServer, error) {
	if cfg.Connector == nil {
		cfg.Connector = db.DefaultConnection
	}
	if err := setup.DB(ctx, cfg.Connector, cfg.NumConns); err != nil {
		return WorkServer{}, err
	}
	if cfg.StuckJobTimeout == 0 {
		cfg.StuckJobTimeout = 7 * time.Minute
	}
	var logger log.Logger
	if cfg.Logger != nil {
		logger = cfg.Logger
	} else {
		logger = log.New()
	}
	return WorkServer{
		processor:           cfg.Processor,
		stuckJobTimeout:     cfg.StuckJobTimeout,
		Logger:              logger.New("svc", "dequeuer"),
		disableMetaShutdown: cfg.DisableMetaShutdown,
	}, nil
}

// How long to wait before marking a job as "stuck"
const DefaultStuckJobTimeout = 7 * time.Minute

var errMetaShutdown = errors.New("dequeuer: received meta.shutdown request")

func (w *WorkServer) run(ctx context.Context) error {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	group, errctx := errgroup.WithContext(cctx)
	group.Go(func() error {
		pools, err := CreatePools(ctx, w.processor, 200*time.Millisecond)
		if err != nil {
			return err
		}
		w.Info("started all worker pools", "jobs", len(pools), "workers", pools.NumDequeuers())
		<-errctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, p := range pools {
			go func(p *Pool) {
				p.Shutdown(shutdownCtx)
			}(p)
		}
		return nil
	})
	if w.metaShutdownEnabled() {
		group.Go(func() error {
			return runShutdownWorker(errctx, w.Logger, cancel)
		})
	}
	group.Go(func() error {
		setup.MeasureActiveQueries(errctx, 1*time.Second)
		return nil
	})
	group.Go(func() error {
		setup.MeasureQueueDepth(errctx, 5*time.Second)
		return nil
	})
	group.Go(func() error {
		setup.MeasureInProgressJobs(errctx, 1*time.Second)
		return nil
	})
	group.Go(func() error {
		// Every minute, check for in-progress jobs that haven't been updated for
		// 7 minutes, and mark them as failed.
		services.WatchStuckJobs(errctx, w.processor, 1*time.Minute, w.stuckJobTimeout)
		return nil
	})
	return group.Wait()
}

// Run starts the WorkServer and several daemons (to measure queue depth,
// process "stuck" jobs)
func (w *WorkServer) Run(ctx context.Context) error {
	for {
		err := w.run(ctx)
		if err == errMetaShutdown && w.metaShutdownEnabled() {
			continue
		}
		return err
	}
}

func (w WorkServer) metaShutdownEnabled() bool {
	return !w.disableMetaShutdown
}

func NewPool(ctx context.Context, name string) *Pool {
	tctx, cancel := context.WithCancel(ctx)
	return &Pool{
		Name:   name,
		ctx:    tctx,
		cancel: cancel,
	}
}

type Pools []*Pool

// NumDequeuers returns the total number of dequeuers across all pools.
func (ps Pools) NumDequeuers() int {
	dequeuerCount := 0
	for _, pool := range ps {
		dequeuerCount = dequeuerCount + len(pool.Dequeuers)
	}
	return dequeuerCount
}

type shutdownWorker struct {
	log.Logger
	cancel   context.CancelFunc
	canceled *int32
}

func (s shutdownWorker) Sleep(failedAttempts int32) time.Duration {
	return time.Second
}

func (s shutdownWorker) DoWork(ctx context.Context, job *newmodels.QueuedJob) error {
	err := services.HandleStatusCallback(ctx, s.Logger, job.ID, job.Name, newmodels.ArchivedJobStatusSucceeded, job.Attempts, false)
	if err != nil {
		s.Warn("received shutdown request but could not update queued job in database. canceling dequeuer anyway", "id", job.ID.String(), "err", err)
	}
	atomic.StoreInt32(s.canceled, 1)
	s.Info("received shutdown request, canceling all dequeuers...")
	s.cancel()
	return nil
}

func runShutdownWorker(ctx context.Context, logger log.Logger, cancel context.CancelFunc) error {
	p := NewPool(ctx, "meta.shutdown")
	canceled := int32(0)
	sw := &shutdownWorker{Logger: logger, cancel: cancel, canceled: &canceled}
	err := p.AddDequeuer(ctx, sw)
	if err != nil {
		return err
	}
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p.Shutdown(shutdownCtx)
	if canceled := atomic.LoadInt32(sw.canceled); canceled == 1 {
		return errMetaShutdown
	}
	return nil
}

// CreatePools creates job pools for all jobs in the database. The provided
// Worker w will be shared between all dequeuers, so it must be thread safe.
func CreatePools(ctx context.Context, w Worker, maxInitialJitter time.Duration) (Pools, error) {
	jobs, err := jobs.GetAll()
	if err != nil {
		return Pools{}, err
	}

	pools := make([]*Pool, len(jobs))
	var g errgroup.Group
	for i, job := range jobs {
		// Copy these so we don't have a concurrency/race problem when the
		// counter iterates
		i := i
		name := job.Name
		concurrency := job.Concurrency
		g.Go(func() error {
			p := NewPool(ctx, name)
			var innerg errgroup.Group
			for j := int16(0); j < concurrency; j++ {
				innerg.Go(func() error {
					time.Sleep(time.Duration(rand.Float64()) * maxInitialJitter)
					err := p.AddDequeuer(ctx, w)
					if err != nil {
						w.Error("could not add dequeuer", "err", err)
					}
					return err
				})
			}
			if err := innerg.Wait(); err != nil {
				return err
			}
			pools[i] = p
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return pools, nil
}

// A Pool contains an array of dequeuers, all of which perform work for the
// same models.Job.
type Pool struct {
	Dequeuers []*Dequeuer
	Name      string
	mu        sync.Mutex
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

type Dequeuer struct {
	log.Logger
	ID     int
	W      Worker
	ctx    context.Context
	cancel context.CancelFunc
}

// A Worker does some work with a QueuedJob. Worker implementations may be
// shared and should be threadsafe.
type Worker interface {
	log.Logger
	// DoWork is responsible for performing work and either updating the job
	// status in the database or waiting for the status to be updated by
	// another thread. Success and failure for the job are marked by hitting
	// services.HandleStatusCallback, or POST /v1/jobs/:job-name/:job-id (over
	// HTTP).
	//
	// A good pattern is for DoWork to make a HTTP request to a downstream
	// service, and then for that service to make a HTTP callback to report
	// success or failure.
	//
	// The Worker is responsible for returning an error if the ExpiresAt
	// deadline is exceeded while the work is in progress.
	//
	// If DoWork is unable to get the work to be done, it should call
	// HandleStatusCallback with a failed callback; errors are logged, but
	// otherwise nothing else is done with them.
	DoWork(context.Context, *newmodels.QueuedJob) error

	// Sleep returns the amount of time to sleep between failed attempts to
	// acquire a queued job. The default implementation sleeps for 20, 40, 80,
	// 160, ..., up to a maximum of 10 seconds between attempts.
	Sleep(failedAttempts int32) time.Duration
}

// AddDequeuer adds a Dequeuer to the Pool and starts running it in a separate
// goroutine. w should be the work that the Dequeuer will do with a dequeued
// job.
func (p *Pool) AddDequeuer(ctx context.Context, w Worker) error {
	select {
	case <-ctx.Done():
		return errPoolShutdown
	default:
	}
	tctx, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	defer p.mu.Unlock()
	d := &Dequeuer{
		Logger: w,
		ID:     len(p.Dequeuers) + 1,
		W:      w,
		ctx:    tctx,
		cancel: cancel,
	}
	p.Dequeuers = append(p.Dequeuers, d)
	p.wg.Add(1)
	go func() {
		d.Work(p.Name, &p.wg)
		// work returned, so it won't do anything more - no point in keeping the
		// dequeuer around
		p.RemoveDequeuer()
	}()
	return nil
}

var errEmptyPool = errors.New("dequeuer: no workers left to dequeue")
var errPoolShutdown = errors.New("dequeuer: cannot add worker because the pool is shutting down")

// RemoveDequeuer removes a dequeuer from the pool and sends that dequeuer
// a shutdown signal.
func (p *Pool) RemoveDequeuer() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.Dequeuers) == 0 {
		return errEmptyPool
	}
	dq := p.Dequeuers[0]
	dq.cancel()
	p.Dequeuers = append(p.Dequeuers[:0], p.Dequeuers[1:]...)
	return nil
}

func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.Dequeuers)
}

// Shutdown all workers in the pool.
func (p *Pool) Shutdown(ctx context.Context) error {
	p.cancel()
	l := p.Len()
	for i := 0; i < l; i++ {
		err := p.RemoveDequeuer()
		if err != nil {
			return err
		}
	}
	done := make(chan struct{}, 1)
	go func() {
		p.wg.Wait()
		done <- struct{}{}
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Dequeuer) Work(name string, wg *sync.WaitGroup) {
	defer wg.Done()
	failedAcquireCount := int32(0)
	waitDuration := time.Duration(0)
	for {
		select {
		case <-d.ctx.Done():
			d.Info("worker quitting", "name", name, "id", d.ID)
			return

		case <-time.After(waitDuration):
			start := time.Now()
			qj, err := queued_jobs.Acquire(context.TODO(), name, d.ID)
			metrics.Time("acquire.latency", time.Since(start))
			if err == nil {
				failedAcquireCount = 0
				waitDuration = time.Duration(0)
				err = d.W.DoWork(d.ctx, qj)
				if err != nil {
					d.Error("could not process job", "id", qj.ID.String(), "err", err)
					metrics.Increment("dequeue." + name + ".error")
					metrics.Increment("dequeue.error")
				} else {
					metrics.Increment("dequeue." + name + ".success")
					metrics.Increment("dequeue.success")
				}
			} else {
				failedAcquireCount++
				waitDuration = d.W.Sleep(failedAcquireCount)
			}
		}
	}
}
