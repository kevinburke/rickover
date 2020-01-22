// Command dequeuer dequeues jobs and sends them to a downstream server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	metrics "github.com/kevinburke/go-simple-metrics"
	"github.com/kevinburke/rickover/config"
	"github.com/kevinburke/rickover/dequeuer"
	"github.com/kevinburke/rickover/services"
	"golang.org/x/sys/unix"
)

func checkError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigterm := make(chan os.Signal, 1)
		signal.Notify(sigterm, unix.SIGINT, unix.SIGTERM)
		sig := <-sigterm
		fmt.Printf("Caught signal %v, shutting down...\n", sig)
		cancel()
	}()
	dbConns, err := config.GetInt("PG_WORKER_POOL_SIZE")
	if err != nil {
		log.Printf("Error getting database pool size: %s. Defaulting to 20", err)
		dbConns = 20
	}

	// We're going to make a lot of requests to the same downstream service.
	httpConns, err := config.GetInt("HTTP_MAX_IDLE_CONNS")
	if err == nil {
		config.SetMaxIdleConnsPerHost(httpConns)
	} else {
		config.SetMaxIdleConnsPerHost(100)
	}

	metrics.Namespace = "rickover.dequeuer"
	metrics.Start("worker", os.Getenv("LIBRATO_EMAIL_ACCOUNT"))

	parsedUrl := config.GetURLOrBail("DOWNSTREAM_URL")
	downstreamPassword := os.Getenv("DOWNSTREAM_WORKER_AUTH")
	if downstreamPassword == "" {
		log.Printf("No DOWNSTREAM_WORKER_AUTH configured, setting an empty password for auth")
	}
	handler := services.NewDownstreamHandler(parsedUrl.String(), downstreamPassword)
	srv, err := dequeuer.New(ctx, dequeuer.Config{
		NumConns:        dbConns,
		Processor:       services.NewJobProcessor(handler),
		StuckJobTimeout: dequeuer.DefaultStuckJobTimeout,
	})
	checkError(err)

	if err := srv.Run(ctx); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
	log.Println("All pools shut down. Quitting.")
}
