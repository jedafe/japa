-- 000651_extend_intel_sources.up.sql
-- Extend intel_sources with public source directory fields.
-- Ref: docs/superpowers/specs/2026-09-11-sources-directory-design.md

BEGIN;

ALTER TABLE intel_sources
    ADD COLUMN IF NOT EXISTS slug text,
    ADD COLUMN IF NOT EXISTS description text,
    ADD COLUMN IF NOT EXISTS logo_url text,
    ADD COLUMN IF NOT EXISTS follower_count int NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS post_count int NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS source_kind text NOT NULL DEFAULT 'publisher'
        CHECK (source_kind IN ('publisher','institution','individual','aggregator'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_intel_sources_tenant_slug
    ON intel_sources (tenant_id, slug) WHERE slug IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_intel_sources_category_kind
    ON intel_sources (tenant_id, category, source_kind) WHERE enabled = true AND status = 'active';

COMMIT;
