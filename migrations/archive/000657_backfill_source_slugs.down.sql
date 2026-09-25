-- 000657_backfill_source_slugs.down.sql
-- Down-migration: we cannot reliably restore the previous NULL slugs, so this
-- only drops the backfilled values. In production, prefer a restore from backup
-- rather than running the down migration.

BEGIN;

UPDATE intel_sources
SET slug = NULL
WHERE id IN (
    SELECT id
    FROM intel_sources
    WHERE slug ~ '^[a-z0-9-]+-[a-f0-9]{6}$'
);

COMMIT;
