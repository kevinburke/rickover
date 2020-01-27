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
	"os"
	"os/signal"

	log "github.com/inconshreveable/log15"
	metrics "github.com/kevinburke/go-simple-metrics"
	"github.com/kevinburke/rickover/config"
	"github.com/kevinburke/rickover/dequeuer"
	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/services"
	"golang.org/x/sys/unix"
)

var dbConns int
var downstreamUrl string
var downstreamPassword string

func init() {
	var err error
	dbConns, err = config.GetInt("PG_WORKER_POOL_SIZE")
	if err != nil {
		log.Info("Error getting database pool size: %s. Defaulting to 20", err)
		dbConns = 20
	}

	downstreamPassword = os.Getenv("DOWNSTREAM_WORKER_AUTH")
	metrics.Namespace = "rickover.dequeuer"
}

func Example_dequeuer() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigterm := make(chan os.Signal, 1)
		signal.Notify(sigterm, unix.SIGINT, unix.SIGTERM)
		sig := <-sigterm
		fmt.Printf("Caught signal %v, shutting down...\n", sig)
		cancel()
	}()

	logger := log.New()
	downstreamUrl = config.GetURLOrBail("DOWNSTREAM_URL").String()
	jp := services.NewJobProcessor(services.NewDownstreamHandler(logger, downstreamUrl, downstreamPassword))

	metrics.Start("worker", "TODO@example.com")

	srv, err := dequeuer.New(ctx, dequeuer.Config{
		Connector:       db.DefaultConnection,
		Processor:       jp,
		NumConns:        10,
		StuckJobTimeout: dequeuer.DefaultStuckJobTimeout,
	})
	if err != nil {
		log.Info("could not start dequeuer", "err", err)
	}
	// Run will:
	//
	// - start all worker pools
	// - start a daemon to "fail" stuck jobs after 7 minutes
	// - start metrics to monitor in progress jobs, active queries against the
	// database, and the depth of the queue.
	if err := srv.Run(ctx); err != nil && err != context.Canceled {
		log.Error("error running dequeuer", "err", err)
		os.Exit(2)
	}
	log.Info("All pools shut down. Quitting.")
}
