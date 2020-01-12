-- name: Truncate :exec
DELETE FROM archived_jobs; DELETE FROM queued_jobs; DELETE FROM jobs;
