-- 000671_restore_video_sources_constraints.up.sql
-- Migration 000668 (final de-tenant) dropped video_sources.tenant_id with
-- CASCADE, which silently removed the UNIQUE(tenant_id, source_url)
-- constraint and the tenant-prefixed indexes. Without a unique constraint on
-- source_url, the CreateOrUpdateVideoSource query's ON CONFLICT (source_url)
-- fails with SQLSTATE 42P10 and every POST /api/v1/videos/resolve and
-- POST /api/v1/videos/upload returns 500.
--
-- The same CASCADE also dropped idx_video_import_jobs_user and
-- idx_video_channel_claims_source_id (list-query indexes, not correctness).
--
-- This migration dedupes any rows that may have been written while the
-- constraint was absent (keeps the most recently updated row per URL),
-- then re-adds the de-tenant unique constraint and the lost indexes.
--
-- Ref: .jeos/sections/106-video-content-and-import.md

BEGIN;

-- Remove duplicate source_url rows, keeping the most recently updated row.
-- Duplicates could only have been written while the unique constraint was
-- absent (between 000668 and this migration). Re-point dependent rows first
-- so the FK cleanup stays consistent.
UPDATE contents c
SET video_source_id = keep.id
FROM video_sources keep, video_sources dup
WHERE c.video_source_id = dup.id
  AND keep.source_url = dup.source_url
  AND (keep.updated_at, keep.id) > (dup.updated_at, dup.id);

UPDATE video_channel_claims vc
SET video_source_id = keep.id
FROM video_sources keep, video_sources dup
WHERE vc.video_source_id = dup.id
  AND keep.source_url = dup.source_url
  AND (keep.updated_at, keep.id) > (dup.updated_at, dup.id);

DELETE FROM video_sources a
USING video_sources b
WHERE a.source_url = b.source_url
  AND (a.updated_at, a.id) < (b.updated_at, b.id);

ALTER TABLE video_sources
    ADD CONSTRAINT video_sources_source_url_key UNIQUE (source_url);

CREATE UNIQUE INDEX IF NOT EXISTS idx_video_sources_external_id
    ON video_sources (source_type, external_id)
    WHERE external_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_video_sources_author
    ON video_sources (author_user_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_video_import_jobs_user
    ON video_import_jobs (user_id, status);

CREATE INDEX IF NOT EXISTS idx_video_channel_claims_source_id
    ON video_channel_claims (video_source_id)
    WHERE video_source_id IS NOT NULL;

COMMIT;
