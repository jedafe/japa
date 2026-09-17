-- 000673_restore_trending_snapshots_unique.down.sql
-- Reverses 000673: drops the restored unique index and the two de-tenant
-- indexes. (This restores the broken state — trending upserts will 42P10
-- again — which is what a true reversal of this fix means.)

BEGIN;

DROP INDEX IF EXISTS trending_snapshots_entity_window_geo_key;
DROP INDEX IF EXISTS idx_trending_snapshots_entity;
DROP INDEX IF EXISTS idx_trending_snapshots_trending;

COMMIT;
