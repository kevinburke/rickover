package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/kevinburke/go-types"
	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/setup"
	"github.com/kevinburke/rickover/test/factory"
	"github.com/kevinburke/semaphore"
)

func main() {
	concurrency := flag.Int("concurrency", 8, "Concurrency to use")
	name := flag.String("name", "", "Job name")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := setup.DB(ctx, db.DefaultConnection, 10); err != nil {
		log.Fatal(err)
	}
	if err := newmodels.DB.Truncate(context.Background()); err != nil {
		log.Fatal(err)
	}
	if *name == "" {
		*name = factory.RandomId("").String()[:8]
	}
	job, err := newmodels.DB.CreateJob(ctx, newmodels.CreateJobParams{
		Name:             *name,
		Concurrency:      int16(*concurrency),
		DeliveryStrategy: newmodels.DeliveryStrategyAtLeastOnce,
		Attempts:         1,
	})
	if err != nil {
		log.Fatal(err)
	}
	data, _ := json.Marshal(factory.RD)
	var wg sync.WaitGroup
	sem := semaphore.New(16)
	// Idea here is basically to insert enough jobs that the dequeuer always has
	// work to do, no matter how long the benchmark takes to run.
	count := 50000
	for j := 0; j < count; j++ {
		sem.Acquire()
		wg.Add(1)
		go func() {
			defer sem.Release()
			defer wg.Done()
			expiresAt := types.NullTime{Valid: false}
			runAfter := time.Now().UTC()
			err := queued_jobs.EnqueueFast(newmodels.EnqueueJobFastParams{
				Name: job.Name, RunAfter: runAfter, ExpiresAt: expiresAt, Data: data,
			})
			if err != nil {
				log.Fatal(err)
			}
		}()
	}
	wg.Wait()
	fmt.Println("wrote", count, "queued jobs")
}
