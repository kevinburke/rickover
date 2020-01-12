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
