-- +goose Up
ALTER TABLE queued_jobs ADD COLUMN auto_id BIGSERIAL;
ALTER TABLE archived_jobs ADD COLUMN auto_id BIGSERIAL;
ALTER TABLE jobs ADD COLUMN auto_id BIGSERIAL;

-- +goose Down

ALTER TABLE queued_jobs DROP COLUMN auto_id;
ALTER TABLE archived_jobs DROP COLUMN auto_id;
ALTER TABLE jobs DROP COLUMN auto_id;
