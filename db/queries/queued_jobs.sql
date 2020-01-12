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

-- name: DeleteQueuedJob :exec
DELETE FROM queued_jobs WHERE id = $1;

-- name: AcquireJob :one
WITH queued_job as (
    SELECT id AS inner_id
    FROM queued_jobs
    WHERE status='queued'
    AND queued_jobs.name = $1
    AND run_after <= now()
    ORDER BY created_at ASC
    LIMIT 1
)
SELECT *
FROM queued_jobs
WHERE pg_try_advisory_lock(queued_job.id);
