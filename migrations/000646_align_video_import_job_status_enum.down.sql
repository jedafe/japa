-- Roll back to the 000640 enum that also allows 'running'.
BEGIN;

ALTER TABLE video_import_jobs
DROP CONSTRAINT IF EXISTS video_import_jobs_status_check,
ADD CONSTRAINT video_import_jobs_status_check
    CHECK (status IN ('pending','processing','running','completed','failed','partial'));

COMMIT;
