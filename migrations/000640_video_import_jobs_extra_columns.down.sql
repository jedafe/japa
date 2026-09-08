-- 000640_video_import_jobs_extra_columns.down.sql
-- Revert the extra columns added to video_import_jobs.

ALTER TABLE video_import_jobs
    DROP CONSTRAINT IF EXISTS video_import_jobs_status_check,
    ADD CONSTRAINT video_import_jobs_status_check
        CHECK (status IN ('pending','processing','completed','failed','partial'));

ALTER TABLE video_import_jobs
    DROP COLUMN IF EXISTS auto_publish,
    DROP COLUMN IF EXISTS started_at;
