-- 000341_create_tax_calculator_tables.up.sql
-- Tax & Financial Obligations Calculator (§100, ADR-108)
-- — tax_treaty_pairs + tax_obligation_guides.
--
-- Adaptation: §100 references sites(id), geo_countries(id). Per ADR-053,
-- tenancy uses tenants(id) (uuid), countries are spaces with level='country'.
--
-- Ref: japaconnect-engineering/sections/100-tax-calculator.md

BEGIN;

CREATE TABLE IF NOT EXISTS tax_treaty_pairs (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    origin_space_id         uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    destination_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    treaty_exists           boolean NOT NULL,
    treaty_summary          text,
    treaty_source_url       text,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, origin_space_id, destination_space_id)
);

CREATE TABLE IF NOT EXISTS tax_obligation_guides (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    origin_space_id         uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    destination_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    employment_status       text NOT NULL, -- employed|self_employed|contractor|unemployed
    residency_trigger       text,
    worldwide_income_taxed  boolean,
    first_year_obligations  text,
    action_checklist        jsonb, -- [{action, deadline_description, official_url}]
    status                  text NOT NULL DEFAULT 'published', -- draft|ai_draft|pending|published|rejected
    is_active               boolean NOT NULL DEFAULT true,
    ai_generated            boolean NOT NULL DEFAULT false,
    ai_model                text,
    ai_prompt_hash          text,
    hitl_reviewed_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    hitl_reviewed_at        timestamptz,
    hitl_review_notes       text,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, origin_space_id, destination_space_id, employment_status)
);

CREATE INDEX IF NOT EXISTS idx_tax_obligation_guides_tenant_dest
    ON tax_obligation_guides(tenant_id, destination_space_id, is_active);

COMMIT;
