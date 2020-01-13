// Code generated by sqlc. DO NOT EDIT.
// source: queued_jobs.sql

package newmodels

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kevinburke/go-types"
)

const acquireJob = `-- name: AcquireJob :one
WITH queued_job_id as (
    SELECT id AS inner_id,
        auto_id as hash_key
    FROM queued_jobs
    WHERE status = 'queued'
    AND queued_jobs.name = $1
    AND run_after <= now()
    ORDER BY created_at ASC
    LIMIT 1
)
SELECT queued_jobs.id, queued_jobs.name, queued_jobs.attempts, queued_jobs.run_after, queued_jobs.expires_at, queued_jobs.created_at, queued_jobs.updated_at, queued_jobs.status, queued_jobs.data, queued_jobs.auto_id
FROM queued_jobs
INNER JOIN queued_job_id ON queued_jobs.id = queued_job_id.inner_id
WHERE id = queued_job_id.inner_id
AND pg_try_advisory_lock(queued_job_id.hash_key)
`

func (q *Queries) AcquireJob(ctx context.Context, name string) (QueuedJob, error) {
	row := q.db.QueryRowContext(ctx, acquireJob, name)
	var i QueuedJob
	err := row.Scan(
		&i.ID,
		&i.Name,
		&i.Attempts,
		&i.RunAfter,
		&i.ExpiresAt,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.Status,
		&i.Data,
		&i.AutoID,
	)
	return i, err
}

const countReadyAndAll = `-- name: CountReadyAndAll :one
WITH all_count AS (
	SELECT count(*) FROM queued_jobs
), ready_count AS (
	SELECT count(*) FROM queued_jobs WHERE run_after <= now()
)
SELECT all_count.count as all, ready_count.count as ready
FROM all_count, ready_count
`

type CountReadyAndAllRow struct {
	All   int64 `json:"all"`
	Ready int64 `json:"ready"`
}

func (q *Queries) CountReadyAndAll(ctx context.Context) (CountReadyAndAllRow, error) {
	row := q.db.QueryRowContext(ctx, countReadyAndAll)
	var i CountReadyAndAllRow
	err := row.Scan(&i.All, &i.Ready)
	return i, err
}

const decrementQueuedJob = `-- name: DecrementQueuedJob :one
UPDATE queued_jobs
SET status = 'queued',
	updated_at = now(),
	attempts = attempts - 1,
	run_after = $3
WHERE id = $1
	AND attempts=$2
	RETURNING id, name, attempts, run_after, expires_at, created_at, updated_at, status, data, auto_id
`

type DecrementQueuedJobParams struct {
	ID       types.PrefixUUID `json:"id"`
	Attempts int16            `json:"attempts"`
	RunAfter time.Time        `json:"run_after"`
}

func (q *Queries) DecrementQueuedJob(ctx context.Context, arg DecrementQueuedJobParams) (QueuedJob, error) {
	row := q.db.QueryRowContext(ctx, decrementQueuedJob, arg.ID, arg.Attempts, arg.RunAfter)
	var i QueuedJob
	err := row.Scan(
		&i.ID,
		&i.Name,
		&i.Attempts,
		&i.RunAfter,
		&i.ExpiresAt,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.Status,
		&i.Data,
		&i.AutoID,
	)
	return i, err
}

const deleteQueuedJob = `-- name: DeleteQueuedJob :many
DELETE FROM queued_jobs
WHERE id = $1
RETURNING id as rows_deleted
`

func (q *Queries) DeleteQueuedJob(ctx context.Context, id types.PrefixUUID) ([]types.PrefixUUID, error) {
	rows, err := q.db.QueryContext(ctx, deleteQueuedJob, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []types.PrefixUUID
	for rows.Next() {
		var rows_deleted types.PrefixUUID
		if err := rows.Scan(&rows_deleted); err != nil {
			return nil, err
		}
		items = append(items, rows_deleted)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const enqueueJob = `-- name: EnqueueJob :one
INSERT INTO queued_jobs (id,
	name,
	attempts,
	run_after,
	expires_at,
	status,
	data)
SELECT $1, jobs.name, attempts, $3, $4, 'queued', $5
FROM jobs
WHERE jobs.name = $2
AND NOT EXISTS (
	SELECT id FROM archived_jobs WHERE id = $1
)
RETURNING id, name, attempts, run_after, expires_at, created_at, updated_at, status, data, auto_id
`

type EnqueueJobParams struct {
	ID        types.PrefixUUID `json:"id"`
	Name      string           `json:"name"`
	RunAfter  time.Time        `json:"run_after"`
	ExpiresAt types.NullTime   `json:"expires_at"`
	Data      json.RawMessage  `json:"data"`
}

func (q *Queries) EnqueueJob(ctx context.Context, arg EnqueueJobParams) (QueuedJob, error) {
	row := q.db.QueryRowContext(ctx, enqueueJob,
		arg.ID,
		arg.Name,
		arg.RunAfter,
		arg.ExpiresAt,
		arg.Data,
	)
	var i QueuedJob
	err := row.Scan(
		&i.ID,
		&i.Name,
		&i.Attempts,
		&i.RunAfter,
		&i.ExpiresAt,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.Status,
		&i.Data,
		&i.AutoID,
	)
	return i, err
}

const getOldInProgressJobs = `-- name: GetOldInProgressJobs :many
SELECT id, name, attempts, run_after, expires_at, created_at, updated_at, status, data, auto_id
FROM queued_jobs
WHERE status = 'in-progress'
AND updated_at < $1
LIMIT 100
`

func (q *Queries) GetOldInProgressJobs(ctx context.Context, updatedAt time.Time) ([]QueuedJob, error) {
	rows, err := q.db.QueryContext(ctx, getOldInProgressJobs, updatedAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []QueuedJob
	for rows.Next() {
		var i QueuedJob
		if err := rows.Scan(
			&i.ID,
			&i.Name,
			&i.Attempts,
			&i.RunAfter,
			&i.ExpiresAt,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.Status,
			&i.Data,
			&i.AutoID,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getQueuedCountsByStatus = `-- name: GetQueuedCountsByStatus :many
SELECT name, count(*)
FROM queued_jobs
WHERE status = $1
GROUP BY name
`

type GetQueuedCountsByStatusRow struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

func (q *Queries) GetQueuedCountsByStatus(ctx context.Context, status JobStatus) ([]GetQueuedCountsByStatusRow, error) {
	rows, err := q.db.QueryContext(ctx, getQueuedCountsByStatus, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetQueuedCountsByStatusRow
	for rows.Next() {
		var i GetQueuedCountsByStatusRow
		if err := rows.Scan(&i.Name, &i.Count); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getQueuedJob = `-- name: GetQueuedJob :one
SELECT id, name, attempts, run_after, expires_at, created_at, updated_at, status, data, auto_id
FROM queued_jobs
WHERE id = $1
`

func (q *Queries) GetQueuedJob(ctx context.Context, id types.PrefixUUID) (QueuedJob, error) {
	row := q.db.QueryRowContext(ctx, getQueuedJob, id)
	var i QueuedJob
	err := row.Scan(
		&i.ID,
		&i.Name,
		&i.Attempts,
		&i.RunAfter,
		&i.ExpiresAt,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.Status,
		&i.Data,
		&i.AutoID,
	)
	return i, err
}

const markInProgress = `-- name: MarkInProgress :one
UPDATE queued_jobs
SET status = 'in-progress',
    updated_at = now()
WHERE id = $1
RETURNING id, name, attempts, run_after, expires_at, created_at, updated_at, status, data, auto_id
`

func (q *Queries) MarkInProgress(ctx context.Context, id types.PrefixUUID) (QueuedJob, error) {
	row := q.db.QueryRowContext(ctx, markInProgress, id)
	var i QueuedJob
	err := row.Scan(
		&i.ID,
		&i.Name,
		&i.Attempts,
		&i.RunAfter,
		&i.ExpiresAt,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.Status,
		&i.Data,
		&i.AutoID,
	)
	return i, err
}