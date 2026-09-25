-- 000325_create_visa_routes.up.sql
-- Visa Route Comparison Tool (§90, ADR-098) — visa_routes table.
--
-- Adaptation: §90 references sites(id), geo_countries(id), visa_categories(id).
-- Per ADR-053, tenancy uses tenants(id) (uuid), countries are spaces with
-- level='country', and no visa_categories table exists — visa category is
-- stored as nullable text.
--
-- ADR-088 standard columns (ai_generated, ai_model, hitl_reviewed_by,
-- ai_prompt_hash, hitl_review_notes) included for universal AI content
-- generation. is_japabot_indexed gates RAG embedding.
--
-- Ref: japaconnect-engineering/sections/90-visa-route-comparison.md
--      japaconnect-engineering/adr/ADR-098-visa-route-comparison.md

BEGIN;

CREATE TABLE IF NOT EXISTS visa_routes (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    destination_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    visa_category           text NOT NULL,
    name                    text NOT NULL,
    slug                    text NOT NULL,
    eligibility_summary     text,
    requirements_json       jsonb,
    typical_fee_usd         numeric(8,2),
    path_to_pr              text,
    renewal_terms           text,
    restrictions            text,
    gov_source_url          text NOT NULL,
    source_url_last_checked date,
    status                  text NOT NULL DEFAULT 'published', -- draft|ai_draft|pending|published|rejected
    is_active               boolean NOT NULL DEFAULT true,
    ai_generated            boolean NOT NULL DEFAULT false,
    ai_model                text,
    ai_prompt_hash          text,
    hitl_reviewed_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    hitl_reviewed_at        timestamptz,
    hitl_review_notes       text,
    is_japabot_indexed      boolean NOT NULL DEFAULT false,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, destination_space_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_visa_routes_tenant_dest_active
    ON visa_routes(tenant_id, destination_space_id, is_active);
CREATE INDEX IF NOT EXISTS idx_visa_routes_tenant_status
    ON visa_routes(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_visa_routes_tenant_visa_cat
    ON visa_routes(tenant_id, visa_category);

COMMIT;
