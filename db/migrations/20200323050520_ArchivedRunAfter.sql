-- We want this so we can see when to schedule the next job, in the case where
-- a timer is being held by Rickover and not an external process.
-- +goose Up
ALTER TABLE archived_jobs ADD COLUMN run_after TIMESTAMP WITH TIME ZONE;
UPDATE archived_jobs SET run_after = NOW();
ALTER TABLE archived_jobs ALTER COLUMN run_after SET NOT NULL;

-- +goose Down
ALTER TABLE archived_jobs DROP COLUMN IF EXISTS run_after;
