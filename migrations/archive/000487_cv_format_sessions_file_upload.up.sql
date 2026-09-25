-- 000487_cv_format_sessions_file_upload.up.sql
-- Adds file upload metadata to cv_format_sessions (§95, ADR-103).
-- Supports PDF/DOCX file uploads alongside the existing text input path.
-- The handler extracts text from uploaded files and stores it in input_text;
-- these columns preserve the original file for audit and re-download.

BEGIN;

ALTER TABLE cv_format_sessions
    ADD COLUMN IF NOT EXISTS original_filename  text,
    ADD COLUMN IF NOT EXISTS file_storage_path   text,
    ADD COLUMN IF NOT EXISTS file_mime_type      text,
    ADD COLUMN IF NOT EXISTS file_size_bytes     bigint;

COMMENT ON COLUMN cv_format_sessions.original_filename IS 'Original filename of the uploaded CV file (if submitted via file upload)';
COMMENT ON COLUMN cv_format_sessions.file_storage_path  IS 'S3/SeaweedFS object key for the uploaded file (if submitted via file upload)';
COMMENT ON COLUMN cv_format_sessions.file_mime_type     IS 'MIME type of the uploaded file (application/pdf, application/vnd.openxmlformats-officedocument.wordprocessingml.document)';
COMMENT ON COLUMN cv_format_sessions.file_size_bytes    IS 'Size of the uploaded file in bytes';

COMMIT;
