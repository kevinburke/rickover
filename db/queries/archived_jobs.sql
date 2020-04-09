-- name: CreateArchivedJob :one
INSERT INTO archived_jobs (id, name, attempts, status, data, expires_at)
SELECT id, $2, $4, $3, data, expires_at
FROM queued_jobs
WHERE queued_jobs.id = $1
AND name = $2
RETURNING *;

-- name: GetArchivedJob :one
SELECT *
FROM archived_jobs
WHERE id = $1;

-- name: ListArchivedJobs :many
SELECT *
FROM archived_jobs
ORDER BY auto_id DESC
LIMIT $1;

-- name: ListArchivedJobsByName :many
SELECT *
FROM archived_jobs
WHERE name = $1
ORDER BY auto_id DESC
LIMIT $2;

-- name: ListArchivedJobsByStatus :many
SELECT *
FROM archived_jobs
WHERE status = $1
ORDER BY auto_id DESC
LIMIT $2;

-- name: ListArchivedJobsByNameStatus :many
SELECT *
FROM archived_jobs
WHERE name = $1
    AND status = $2
ORDER BY auto_id DESC
LIMIT $3;
