-- +goose Up
-- FYI this index takes up about 1/10th of the table space.
CREATE INDEX CONCURRENTLY archived_jobs_status on archived_jobs (status);

-- +goose Down
DROP INDEX IF EXISTS archived_jobs_status;
