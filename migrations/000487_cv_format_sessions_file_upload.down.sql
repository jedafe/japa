-- 000487_cv_format_sessions_file_upload.down.sql
-- Reverses 000487: removes file upload metadata columns.

BEGIN;

ALTER TABLE cv_format_sessions
    DROP COLUMN IF EXISTS file_size_bytes,
    DROP COLUMN IF EXISTS file_mime_type,
    DROP COLUMN IF EXISTS file_storage_path,
    DROP COLUMN IF EXISTS original_filename;

COMMIT;
