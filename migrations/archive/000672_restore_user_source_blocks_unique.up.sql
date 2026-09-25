-- 000672_restore_user_source_blocks_unique.up.sql
-- Migration 000668 (final de-tenant) dropped user_source_blocks.tenant_id
-- with CASCADE, which silently removed the UNIQUE(tenant_id, blocker_id,
-- source_id) constraint and the tenant-prefixed indexes
-- (idx_user_source_blocks_blocker, idx_user_source_blocks_source). Without a
-- unique constraint on (blocker_id, source_id), the CreateSourceBlock query's
-- ON CONFLICT (blocker_id, source_id) fails with SQLSTATE 42P10 and every
-- POST /api/v1/sources/:slug/block returns 500.
--
-- This migration dedupes any rows that may have been written while the
-- constraint was absent (keeps the most recently created row per pair),
-- then re-adds the de-tenant unique constraint and the lost indexes.
-- user_source_blocks is a leaf table — no FKs point at it, so dedupe needs
-- no dependent re-pointing.
--
-- Ref: .jeos/sections/ (intel sources); precedent: 000670/000671.

BEGIN;

-- Remove duplicate (blocker_id, source_id) rows, keeping the most recently
-- created row per pair.
DELETE FROM user_source_blocks a
USING user_source_blocks b
WHERE a.blocker_id = b.blocker_id
  AND a.source_id = b.source_id
  AND (a.created_at, a.id) < (b.created_at, b.id);

ALTER TABLE user_source_blocks
    ADD CONSTRAINT user_source_blocks_blocker_source_key UNIQUE (blocker_id, source_id);

CREATE INDEX IF NOT EXISTS idx_user_source_blocks_blocker
    ON user_source_blocks (blocker_id);

CREATE INDEX IF NOT EXISTS idx_user_source_blocks_source
    ON user_source_blocks (source_id);

COMMIT;
