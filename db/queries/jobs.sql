-- name: CreateJob :one
INSERT INTO jobs (
    name,
    delivery_strategy,
    attempts,
    concurrency)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetJob :one
SELECT *
FROM jobs
WHERE name = $1;

-- name: GetAllJobs :many
SELECT * FROM jobs;

-- name: DeleteAllJobs :execrows
DELETE FROM jobs;
