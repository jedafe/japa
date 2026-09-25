-- 000651_extend_intel_sources.down.sql
-- Revert intel_sources public source directory extension.

BEGIN;

DROP INDEX IF EXISTS idx_intel_sources_category_kind;
DROP INDEX IF EXISTS idx_intel_sources_tenant_slug;

ALTER TABLE intel_sources
    DROP COLUMN IF EXISTS source_kind,
    DROP COLUMN IF EXISTS post_count,
    DROP COLUMN IF EXISTS follower_count,
    DROP COLUMN IF EXISTS logo_url,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS slug;

COMMIT;
