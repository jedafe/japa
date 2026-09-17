-- 000673_restore_trending_snapshots_unique.up.sql
-- Migration 000668 (final de-tenant) dropped trending_snapshots.tenant_id with
-- CASCADE, which silently removed the UNIQUE(tenant_id, entity_type,
-- entity_id, "window", geo_code) constraint and the tenant-prefixed indexes
-- (idx_trending_snapshots_tenant_entity, idx_trending_snapshots_trending).
-- Without the unique constraint, the UpsertTrendingSnapshot query's
-- ON CONFLICT (entity_type, entity_id, "window", geo_code) fails with
-- SQLSTATE 42P10 and the trending processor errors on every upsert.
--
-- This migration dedupes any rows written while the constraint was absent
-- (keeps the most recently computed row per key), then re-adds the
-- de-tenant unique constraint and the lost indexes. trending_snapshots is a
-- leaf table — no FKs point at it.
--
-- Ref: .jeos/sections/ (trending); precedent: 000670/000671/000672.

BEGIN;

-- geo_code is NULLable; make the dedupe and the constraint treat NULL as a
-- distinct-consistent key by coalescing to '' for comparison.
DELETE FROM trending_snapshots a
USING trending_snapshots b
WHERE a.entity_type = b.entity_type
  AND a.entity_id = b.entity_id
  AND a."window" = b."window"
  AND COALESCE(a.geo_code, '') = COALESCE(b.geo_code, '')
  AND (a.computed_at, a.id) < (b.computed_at, b.id);

-- ON CONFLICT with a NULLable column needs a UNIQUE INDEX with NULLS NOT
-- DISTINCT to dedupe correctly (plain UNIQUE treats NULLs as distinct).
CREATE UNIQUE INDEX trending_snapshots_entity_window_geo_key
    ON trending_snapshots (entity_type, entity_id, "window", geo_code)
    NULLS NOT DISTINCT;

CREATE INDEX IF NOT EXISTS idx_trending_snapshots_entity
    ON trending_snapshots (entity_type, entity_id);

CREATE INDEX IF NOT EXISTS idx_trending_snapshots_trending
    ON trending_snapshots (entity_type, "window", geo_code, score DESC);

COMMIT;
