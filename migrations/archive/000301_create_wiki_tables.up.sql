-- 000301_create_wiki_tables.up.sql
-- Destination Wiki (§80, ADR-088) — wiki_pages + wiki_revisions tables.
--
-- Collaborative, versioned knowledge-base pages attached to each destination
-- country/city Space. Every Country Space receives a wiki index with
-- pre-created sub-pages (Overview, Visa Routes, Cost of Living, Healthcare,
-- Banking, Community, Housing).
--
-- Schema adaptation note: §80 references sites(id) and BIGINT/BIGSERIAL IDs.
-- Per ADR-053, the codebase uses uuid PKs and tenants(id) for tenancy.
--
-- ADR-088 standard columns (ai_generated, ai_model, hitl_reviewed_by,
-- hitl_reviewed_at) are included on wiki_revisions for universal AI content
-- generation. is_japabot_indexed is a generated column gating RAG embedding.
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
--      japaconnect-engineering/adr/ADR-088-destination-wiki.md

BEGIN;

CREATE TABLE IF NOT EXISTS wiki_pages (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    space_id           uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    slug               text NOT NULL,
    title              text NOT NULL,
    category           text NOT NULL DEFAULT 'other', -- overview|visa|cost_of_living|healthcare|banking|community|housing|other
    is_active          boolean NOT NULL DEFAULT true,
    is_featured        boolean NOT NULL DEFAULT false,
    is_locked          boolean NOT NULL DEFAULT false, -- locked to mod-only edits
    created_by         uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    is_japabot_indexed boolean GENERATED ALWAYS AS (
        is_active = true AND is_locked = false
    ) STORED,
    UNIQUE(tenant_id, space_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_tenant_space
    ON wiki_pages(tenant_id, space_id);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_category
    ON wiki_pages(tenant_id, space_id, category);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_featured
    ON wiki_pages(tenant_id, is_featured)
    WHERE is_featured = true;

CREATE TABLE IF NOT EXISTS wiki_revisions (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    wiki_page_id        uuid NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
    content             jsonb NOT NULL DEFAULT '{}', -- TipTap JSON
    summary             text, -- edit summary from submitter
    status              text NOT NULL DEFAULT 'pending', -- pending|approved|rejected
    submitted_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_by         uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at         timestamptz,
    -- ADR-088 universal AI content generation columns
    ai_generated        boolean NOT NULL DEFAULT false,
    ai_model            text,
    ai_prompt_hash      text, -- SHA-256 of the prompt (audit reproducibility)
    hitl_reviewed_by    uuid REFERENCES users(id) ON DELETE SET NULL,
    hitl_reviewed_at    timestamptz,
    hitl_review_notes   text,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wiki_revisions_page_status
    ON wiki_revisions(wiki_page_id, status);
CREATE INDEX IF NOT EXISTS idx_wiki_revisions_tenant_status
    ON wiki_revisions(tenant_id, status, created_at DESC);

COMMIT;
