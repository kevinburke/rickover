-- name: EnqueueJob :one
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
RETURNING *;

-- name: GetQueuedJob :one
SELECT *
FROM queued_jobs
WHERE id = $1;

-- name: DeleteQueuedJob :many
DELETE FROM queued_jobs
WHERE id = $1
RETURNING id as rows_deleted;

-- name: AcquireJob :one
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
SELECT queued_jobs.*
FROM queued_jobs
INNER JOIN queued_job_id ON queued_jobs.id = queued_job_id.inner_id
WHERE id = queued_job_id.inner_id
AND pg_try_advisory_lock(queued_job_id.hash_key);

-- name: OldAcquireJob :one
WITH queued_job as (
	SELECT id AS inner_id
	FROM queued_jobs
	WHERE status='queued'
		AND queued_jobs.name = $1
		AND run_after <= now()
	ORDER BY created_at ASC
	LIMIT 1
	FOR UPDATE
)
UPDATE queued_jobs
SET status='in-progress',
	updated_at=now()
FROM queued_job
WHERE queued_jobs.id = queued_job.inner_id
	AND status='queued'
RETURNING queued_jobs.*;

-- name: MarkInProgress :one
UPDATE queued_jobs
SET status = 'in-progress',
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetQueuedCountsByStatus :many
SELECT name, count(*)
FROM queued_jobs
WHERE status = $1
GROUP BY name;

-- name: GetOldInProgressJobs :many
SELECT *
FROM queued_jobs
WHERE status = 'in-progress'
AND updated_at < $1
LIMIT 100;

-- name: DecrementQueuedJob :one
UPDATE queued_jobs
SET status = 'queued',
	updated_at = now(),
	attempts = attempts - 1,
	run_after = $3
WHERE id = $1
	AND attempts=$2
	RETURNING *;

-- name: CountReadyAndAll :one
WITH all_count AS (
	SELECT count(*) FROM queued_jobs
), ready_count AS (
	SELECT count(*) FROM queued_jobs WHERE run_after <= now()
)
SELECT all_count.count as all, ready_count.count as ready
FROM all_count, ready_count;
