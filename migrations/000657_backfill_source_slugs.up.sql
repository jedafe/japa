-- 000657_backfill_source_slugs.up.sql
-- Backfill slugs for existing intel_sources rows. A slug is derived from the
-- source name with a short random suffix so the (tenant_id, slug) unique index
-- is satisfied without manual collision resolution.
--
-- Ref: docs/superpowers/plans/2026-09-11-sources-directory-plan.md

BEGIN;

UPDATE intel_sources
SET slug = concat(
    trim(both '-' from lower(regexp_replace(name, '[^a-zA-Z0-9]+', '-', 'g'))),
    '-',
    substr(gen_random_uuid()::text, 1, 6)
)
WHERE slug IS NULL;

COMMIT;
