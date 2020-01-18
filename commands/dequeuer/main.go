// Command dequeuer dequeues jobs and sends them to a downstream server.
package main

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

func checkError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	dbConns, err := config.GetInt("PG_WORKER_POOL_SIZE")
	if err != nil {
		log.Printf("Error getting database pool size: %s. Defaulting to 20", err)
		dbConns = 20
	}

	err = setup.DB(context.Background(), db.DefaultConnection, dbConns)
	checkError(err)

	go setup.MeasureActiveQueries(1 * time.Second)
	go setup.MeasureQueueDepth(5 * time.Second)
	go setup.MeasureInProgressJobs(1 * time.Second)

	// Every minute, check for in-progress jobs that haven't been updated for
	// 7 minutes, and mark them as failed.
	go services.WatchStuckJobs(1*time.Minute, 7*time.Minute)

	// We're going to make a lot of requests to the same downstream service.
	httpConns, err := config.GetInt("HTTP_MAX_IDLE_CONNS")
	if err == nil {
		config.SetMaxIdleConnsPerHost(httpConns)
	} else {
		config.SetMaxIdleConnsPerHost(100)
	}

	metrics.Namespace = "rickover.dequeuer"
	metrics.Start("worker", os.Getenv("LIBRATO_EMAIL_ACCOUNT"))

	downstreamPassword := os.Getenv("DOWNSTREAM_WORKER_AUTH")
	if downstreamPassword == "" {
		log.Printf("No DOWNSTREAM_WORKER_AUTH configured, setting an empty password for auth")
	}

	parsedUrl := config.GetURLOrBail("DOWNSTREAM_URL")
	handler := services.NewDownstreamHandler(parsedUrl.String(), downstreamPassword)
	jp := services.NewJobProcessor(handler)

	ctx, cancel := context.WithCancel(context.Background())
	// This creates a pool of dequeuers and starts them.
	pools, err := dequeuer.CreatePools(ctx, jp, 200*time.Millisecond)
	checkError(err)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, unix.SIGINT, unix.SIGTERM)
	sig := <-sigterm
	fmt.Printf("Caught signal %v, shutting down...\n", sig)
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	g, errctx := errgroup.WithContext(shutdownCtx)
	for _, p := range pools {
		if p != nil {
			p := p
			g.Go(func() error {
				err := p.Shutdown(errctx)
				if err != nil {
					log.Printf("Error shutting down pool: %v", err.Error())
				}
				return err
			})
		}
	}
	if err := g.Wait(); err != nil {
		log.Fatal(err)
	}
	log.Println("All pools shut down. Quitting.")
}
