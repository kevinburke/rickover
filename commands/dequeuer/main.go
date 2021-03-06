// Command dequeuer dequeues jobs and sends them to a downstream server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/kevinburke/handlers"
	"github.com/kevinburke/rickover/config"
	"github.com/kevinburke/rickover/dequeuer"
	"github.com/kevinburke/rickover/metrics"
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
	logger := handlers.Logger
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

	go metrics.Run(ctx, metrics.LibratoConfig{
		Namespace: "rickover.dequeuer",
		Source:    "worker",
		Email:     os.Getenv("LIBRATO_EMAIL_ACCOUNT"),
	})

	parsedUrl := config.GetURLOrBail("DOWNSTREAM_URL")
	downstreamPassword := os.Getenv("DOWNSTREAM_WORKER_AUTH")
	if downstreamPassword == "" {
		logger.Warn("No DOWNSTREAM_WORKER_AUTH configured, setting an empty password for auth")
	}
	handler := services.NewDownstreamHandler(logger, parsedUrl.String(), downstreamPassword)
	srv, err := dequeuer.New(ctx, dequeuer.Config{
		Logger:              logger,
		NumConns:            dbConns,
		Processor:           services.NewJobProcessor(handler),
		StuckJobTimeout:     dequeuer.DefaultStuckJobTimeout,
		DisableMetaShutdown: os.Getenv("DISABLE_META_SHUTDOWN") == "true",
	})
	checkError(err)

	if err := srv.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("error running dequeuer", "err", err)
		os.Exit(1)
	}
	logger.Info("All pools shut down. Quitting.")
}
