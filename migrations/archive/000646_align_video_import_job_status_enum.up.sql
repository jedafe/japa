-- §106: the video_import_jobs.status enum is 'pending','processing','completed','failed','partial'.
-- Migration 000640 temporarily added 'running' to the CHECK. Align with the spec.
BEGIN;

UPDATE video_import_jobs
SET status = 'processing'
WHERE status = 'running';

ALTER TABLE video_import_jobs
DROP CONSTRAINT IF EXISTS video_import_jobs_status_check,
ADD CONSTRAINT video_import_jobs_status_check
    CHECK (status IN ('pending','processing','completed','failed','partial'));

COMMIT;
