-- 000670_restore_saved_scholarships_unique.up.sql
-- Migration 000668 (final de-tenant) dropped saved_scholarships.tenant_id with
-- CASCADE, which silently removed the UNIQUE(tenant_id, user_id, guide_id)
-- constraint and the three tenant-prefixed indexes. Without a unique
-- constraint on (user_id, guide_id), the SaveScholarship query's
-- ON CONFLICT (user_id, guide_id) DO NOTHING fails with SQLSTATE 42P10 and
-- every POST /api/v1/scholarships/{id}/save returns 400.
--
-- This migration dedupes any rows that may have been written while the
-- constraint was absent (keeps the most recently updated row per pair),
-- then re-adds the de-tenant unique constraint and the indexes.
--
-- Ref: japaconnect-engineering/sections/73-scholarship-finder.md

BEGIN;

-- Remove duplicate (user_id, guide_id) pairs, keeping the most recently
-- updated row. Duplicates could only have been written while the unique
-- constraint was absent (between 000668 and this migration).
DELETE FROM saved_scholarships a
USING saved_scholarships b
WHERE a.user_id = b.user_id
  AND a.guide_id = b.guide_id
  AND (a.updated_at, a.id) < (b.updated_at, b.id);

ALTER TABLE saved_scholarships
    ADD CONSTRAINT saved_scholarships_user_id_guide_id_key UNIQUE (user_id, guide_id);

CREATE INDEX IF NOT EXISTS idx_saved_scholarships_user
    ON saved_scholarships (user_id, status);

CREATE INDEX IF NOT EXISTS idx_saved_scholarships_deadline
    ON saved_scholarships (guide_id);

CREATE INDEX IF NOT EXISTS idx_saved_scholarships_remind
    ON saved_scholarships (last_reminded_at)
    WHERE last_reminded_at IS NULL;

COMMIT;
