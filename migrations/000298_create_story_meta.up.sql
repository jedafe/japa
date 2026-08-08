-- 000298_create_story_meta.up.sql
-- Community Story Wall (§79, ADR-087) — story_meta companion table.
--
-- Stories are records in the contents table with content_type = 'story'
-- (TEXT column, no enum alteration needed). This extension table holds the
-- structured fields that enable filtered browsing and Meilisearch faceting.
--
-- Schema adaptation note: §79 references geo_countries(id), sites(id), and
-- visa_categories(id). Per ADR-053 (running code is the source of truth),
-- countries are spaces with level='country' (migration 000009/000100),
-- tenancy uses tenants(id), and no visa_categories table exists — visa
-- category is stored as nullable text.
--
-- ADR-088 standard columns (ai_generated, ai_model, hitl_reviewed_by,
-- ai_generated_at) are included for universal AI content generation.
-- is_japabot_indexed gates RAG embedding per ADR-088.
--
-- Ref: japaconnect-engineering/sections/79-community-story-wall.md
--      japaconnect-engineering/adr/ADR-087-community-story-wall.md
--      japaconnect-engineering/adr/ADR-088-universal-ai-content-generation.md

BEGIN;

CREATE TABLE IF NOT EXISTS story_meta (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id              uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id             uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    origin_space_id        uuid REFERENCES spaces(id) ON DELETE SET NULL,
    destination_space_id   uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    relocation_year        smallint,
    visa_category          text,
    profession             text,
    biggest_challenge      text,
    most_useful_resource   text,
    best_advice            text,
    verified               boolean NOT NULL DEFAULT false,
    -- ADR-088 universal AI content generation columns
    ai_generated           boolean NOT NULL DEFAULT false,
    ai_model               text,
    hitl_reviewed_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    ai_generated_at        timestamptz,
    hitl_reviewed_at       timestamptz,
    is_japabot_indexed     boolean NOT NULL DEFAULT false,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_story_meta_tenant_dest
    ON story_meta(tenant_id, destination_space_id);
CREATE INDEX IF NOT EXISTS idx_story_meta_tenant_origin
    ON story_meta(tenant_id, origin_space_id);
CREATE INDEX IF NOT EXISTS idx_story_meta_tenant_visa
    ON story_meta(tenant_id, visa_category)
    WHERE visa_category IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_story_meta_content
    ON story_meta(content_id);
CREATE INDEX IF NOT EXISTS idx_story_meta_verified
    ON story_meta(tenant_id, verified)
    WHERE verified = true;

COMMIT;
