-- 000671_restore_video_sources_constraints.down.sql
-- Reverts 000671: drops the de-tenant unique constraint and indexes on the
-- video tables. (Does NOT restore tenant_id — that is 000668's down.)

BEGIN;

DROP INDEX IF EXISTS idx_video_channel_claims_source_id;
DROP INDEX IF EXISTS idx_video_import_jobs_user;
DROP INDEX IF EXISTS idx_video_sources_author;
DROP INDEX IF EXISTS idx_video_sources_external_id;

ALTER TABLE video_sources
    DROP CONSTRAINT IF EXISTS video_sources_source_url_key;

COMMIT;
