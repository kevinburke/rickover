-- +goose Up
CREATE INDEX CONCURRENTLY archived_jobs_name on archived_jobs (name);

-- +goose Down
DROP INDEX IF EXISTS archived_jobs_name;
