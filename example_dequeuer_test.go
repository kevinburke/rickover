// Run the rickover dequeuer. Configure the following environment variables:
//
// DATABASE_URL: Postgres connection string (see Makefile)
// PG_WORKER_POOL_SIZE: Maximum number of database connections from this process
// DOWNSTREAM_URL: Downstream server that can perform the work
// DOWNSTREAM_WORKER_AUTH: Basic Auth password for downstream server (user "jobs")
//
// Create job types by making a POST request to /v1/jobs with the job name and
// concurrency. After that, CreatePools will start and run dequeuers for those
// types.

package rickover

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	metrics "github.com/kevinburke/go-simple-metrics"
	"github.com/kevinburke/rickover/config"
	"github.com/kevinburke/rickover/dequeuer"
	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/services"
	"github.com/kevinburke/rickover/setup"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
)

var dbConns int
var downstreamUrl string
var downstreamPassword string

func init() {
	var err error
	dbConns, err = config.GetInt("PG_WORKER_POOL_SIZE")
	if err != nil {
		log.Printf("Error getting database pool size: %s. Defaulting to 20", err)
		dbConns = 20
	}

	downstreamPassword = os.Getenv("DOWNSTREAM_WORKER_AUTH")
	metrics.Namespace = "rickover.dequeuer"
}

func Example_dequeuer() {
	if err := setup.DB(context.TODO(), db.DefaultConnection, dbConns); err != nil {
		log.Fatal(err)
	}

	metrics.Start("worker", "TODO@example.com")

	go setup.MeasureActiveQueries(1 * time.Second)
	go setup.MeasureQueueDepth(5 * time.Second)
	go setup.MeasureInProgressJobs(1 * time.Second)

	// Every minute, check for in-progress jobs that haven't been updated for
	// 7 minutes, and mark them as failed.
	go services.WatchStuckJobs(1*time.Minute, 7*time.Minute)

	downstreamUrl = config.GetURLOrBail("DOWNSTREAM_URL").String()
	jp := services.NewJobProcessor(services.NewDownstreamHandler(downstreamUrl, downstreamPassword))

	// CreatePools will read all job types out of the jobs table, then start
	// all dequeuers for those jobs.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	pools, err := dequeuer.CreatePools(ctx, jp, 200*time.Millisecond)
	if err != nil {
		log.Fatal(err)
	}

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, unix.SIGINT, unix.SIGTERM)
	sig := <-sigterm
	fmt.Printf("Caught signal %v, shutting down...\n", sig)
	cancel()
	group, errctx := errgroup.WithContext(ctx)
	for _, p := range pools {
		if p != nil {
			p := p
			group.Go(func() error {
				return p.Shutdown(errctx)
			})
		}
	}
	if err := group.Wait(); err != nil {
		log.Printf("Error shutting down pool: %v", err.Error())
	}
	log.Println("All pools shut down. Quitting.")
}
