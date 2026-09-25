-- 000619_fix_roadmap_variant_geo_metadata.down.sql
-- The up migration repaired variant geo metadata from the slug-encoded pair.
-- The previous (corrupted) column values cannot be reconstructed, so this down
-- migration is a documented no-op. If a rollback is required, restore the
-- affected rows from a database backup taken before the migration.

BEGIN;

SELECT 1;

COMMIT;
