-- +goose Up
ALTER TABLE queued_jobs ADD COLUMN auto_id BIGSERIAL NOT NULL;
CREATE INDEX queued_jobs_auto_id_key ON queued_jobs (auto_id);

ALTER TABLE archived_jobs ADD COLUMN auto_id BIGSERIAL NOT NULL;
ALTER TABLE jobs ADD COLUMN auto_id BIGSERIAL NOT NULL;

-- +goose Down

ALTER TABLE queued_jobs DROP COLUMN auto_id;
ALTER TABLE archived_jobs DROP COLUMN auto_id;
ALTER TABLE jobs DROP COLUMN auto_id;
