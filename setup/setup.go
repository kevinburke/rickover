// Package setup helps initialize the database and all queries.
package setup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	metrics "github.com/kevinburke/go-simple-metrics"
	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
	_ "github.com/lib/pq"
)

var mu sync.Mutex

// TODO not sure for the best place for this to live.
var activeQueriesStmt *sql.Stmt

func prepare(ctx context.Context) (err error) {
	if !db.Connected() {
		return errors.New("setup: no DB connection was established, can't query")
	}

	activeQueriesStmt, err = db.Conn.PrepareContext(ctx, `-- setup.GetActiveQueries
SELECT count(*) FROM pg_stat_activity 
WHERE state='active'
	`)
	return
}

func GetActiveQueries(ctx context.Context) (count int64, err error) {
	err = activeQueriesStmt.QueryRowContext(ctx).Scan(&count)
	return
}

// TODO all of these should use a different database connection than the server
// or the worker, to avoid contention.
func MeasureActiveQueries(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		count, err := GetActiveQueries(ctx)
		if err == nil {
			go metrics.Measure("active_queries.count", count)
		} else {
			go metrics.Increment("active_queries.error")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func MeasureQueueDepth(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		allCount, readyCount, err := queued_jobs.CountReadyAndAll(ctx)
		if err == nil {
			go metrics.Measure("queue_depth.all", int64(allCount))
			go metrics.Measure("queue_depth.ready", int64(readyCount))
		} else {
			go metrics.Increment("queue_depth.error")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func MeasureInProgressJobs(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		m, err := queued_jobs.GetCountsByStatus(ctx, newmodels.JobStatusInProgress)
		if err == nil {
			count := int64(0)
			for k, v := range m {
				count += v
				go metrics.Measure(fmt.Sprintf("queued_jobs.%s.in_progress", k), v)
			}
			go metrics.Measure("queued_jobs.in_progress", count)
		} else {
			go metrics.Increment("queued_jobs.in_progress.error")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// DB initializes a connection to the database, and prepares queries on all
// models. If connector is nil, db.DefaultConnection will be used.
func DB(ctx context.Context, connector db.Connector, dbConns int) error {
	mu.Lock()
	defer mu.Unlock()
	if db.Conn != nil {
		if err := db.Conn.PingContext(ctx); err == nil {
			// Already connected.
			return nil
		}
	}
	var dbConnector db.Connector = connector
	if dbConnector == nil {
		dbConnector = db.DefaultConnection
	}
	conn, err := dbConnector.Connect(dbConns)
	if err != nil {
		return errors.New("setup: could not establish a database connection: " + err.Error())
	}
	db.Conn = conn
	if err := db.Conn.PingContext(ctx); err != nil {
		return errors.New("setup: could not establish a database connection: " + err.Error())
	}
	return PrepareAll(ctx)
}

func PrepareAll(ctx context.Context) error {
	if err := newmodels.Setup(ctx); err != nil {
		return err
	}
	if err := prepare(ctx); err != nil {
		return err
	}
	return nil
}
