-- +goose Up
CREATE INDEX CONCURRENTLY archived_jobs_auto_id ON archived_jobs (auto_id);

-- +goose Down
DROP INDEX IF EXISTS archived_jobs_auto_id;
