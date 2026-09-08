-- 000640_video_import_jobs_extra_columns.up.sql
-- Ref: .jeos/sections/106-video-content-and-import.md
-- Extra columns for bulk YouTube import jobs.

ALTER TABLE video_import_jobs
    ADD COLUMN IF NOT EXISTS auto_publish boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS started_at   timestamptz;

ALTER TABLE video_import_jobs
    DROP CONSTRAINT IF EXISTS video_import_jobs_status_check,
    ADD CONSTRAINT video_import_jobs_status_check
        CHECK (status IN ('pending','processing','running','completed','failed','partial'));
