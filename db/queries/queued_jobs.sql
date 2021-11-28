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
	SELECT 1 FROM archived_jobs WHERE id = $1
)
RETURNING attempts, status, created_at, updated_at;

-- name: EnqueueJobFast :exec
INSERT INTO queued_jobs (id,
	name,
	attempts,
	run_after,
	expires_at,
	status,
	data)
SELECT uuid_generate_v4()::uuid, jobs.name, attempts, $2, $3, 'queued', $4
FROM jobs
WHERE jobs.name = $1;

-- name: GetQueuedJob :one
SELECT *
FROM queued_jobs
WHERE id = $1;

-- name: GetQueuedJobAttempts :one
SELECT attempts
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
    LIMIT 1
)
SELECT id
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
	ORDER BY id
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

-- name: DeleteAllQueuedJobs :execrows
DELETE FROM queued_jobs;


--
-- _name: AcquireJob2 :one
-- WITH queued_job_id as (
    -- SELECT id AS inner_id,
        -- auto_id as hash_key
    -- FROM queued_jobs
    -- WHERE status = 'queued'
    -- AND queued_jobs.name = $1
    -- AND run_after <= now()
    -- ORDER BY id
    -- LIMIT 1
-- ), locked_row AS (
    -- SELECT inner_id as id, pg_try_advisory_lock(queued_job_id.hash_key)
    -- FROM queued_job_id
-- )
-- UPDATE queued_jobs
-- SET status = 'in-progress',
    -- updated_at = now()
-- FROM locked_row
-- WHERE queued_jobs.id = locked_row.id
-- RETURNING *;

--
-- _name: AcquireJobRecursive :one
-- WITH RECURSIVE lock_candidates (n) AS (
    -- -- Pick a bunch of candidate jobs from the database
    -- SELECT * FROM (
        -- SELECT 0, auto_id, id,
        -- row_number() over (order by auto_id) as rownumber, false as locked, 0::bigint as locked_row_id
        -- FROM queued_jobs
        -- WHERE run_after < now()
        -- LIMIT 10
    -- ) t1
  -- UNION ALL (
    -- -- Try to lock each one in turn. The first time we recurse, n+1 = 1, so we
    -- -- try to lock the first row in the CASE WHEN statement.
    -- -- Second time, n+1 = 2, we try to lock the second row in the CASE WHEN
    -- -- statement.
    -- WITH t2 AS (
        -- SELECT lock_candidates.n+1, lock_candidates.auto_id, lock_candidates.id,
            -- lock_candidates.rownumber,
            -- CASE WHEN lock_candidates.n+1 = lock_candidates.rownumber
                -- THEN (pg_try_advisory_lock(lock_candidates.auto_id))
                -- ELSE false
            -- END AS locked
        -- FROM queued_jobs
        -- -- Join so we only check the rows that were pulled by the first query
        -- INNER JOIN lock_candidates ON queued_jobs.auto_id = lock_candidates.auto_id
        -- WHERE lock_candidates.n < 10
    -- ), t2_and_locked_row AS (
        -- -- Put the auto_id of the locked row at the end of every row, we use
        -- -- this to make t3 easier
        -- SELECT t2.*, COALESCE((SELECT auto_id c FROM t2 WHERE locked = true LIMIT 1), 0) locked_row_id FROM t2
    -- )
    -- -- Return either the single locked row OR all of the non-locked rows.
    -- SELECT *
    -- FROM t2_and_locked_row t3
    -- WHERE (locked_row_id > 0 AND t3.auto_id = locked_row_id) OR (
        -- locked_row_id = 0
    -- )
  -- )
-- )
-- SELECT * FROM lock_candidates where locked_row_id > 0 LIMIT 1;
