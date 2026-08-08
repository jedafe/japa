-- 000329_create_arrival_checklist_tables.up.sql
-- Arrival Checklist Generator (§92, ADR-100) — arrival_checklist_guides + arrival_checklist_items.
-- Reuses checklist_progress from §70 with list_type = 'arrival'.
--
-- Adaptation: §92 references sites(id), geo_countries(id), visa_categories(id).
-- Per ADR-053, tenancy uses tenants(id) (uuid), countries are spaces with
-- level='country', and visa category is stored as nullable text.
--
-- Ref: japaconnect-engineering/sections/92-arrival-checklist.md

BEGIN;

CREATE TABLE IF NOT EXISTS arrival_checklist_guides (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    destination_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    visa_category           text, -- NULL = applies to all visa types
    applicant_type          text NOT NULL DEFAULT 'individual', -- individual|couple|family
    version                 int NOT NULL DEFAULT 1,
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
    UNIQUE (tenant_id, destination_space_id, visa_category, applicant_type, version)
);

CREATE INDEX IF NOT EXISTS idx_arrival_checklist_guides_tenant_dest
    ON arrival_checklist_guides(tenant_id, destination_space_id, is_active);

CREATE TABLE IF NOT EXISTS arrival_checklist_items (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    guide_id                uuid NOT NULL REFERENCES arrival_checklist_guides(id) ON DELETE CASCADE,
    phase                   text NOT NULL, -- day_1_7|week_2_4|month_1_3|ongoing
    title                   text NOT NULL,
    description             text,
    official_link           text,
    origin_country_modifier jsonb, -- {space_id: "specific note for this origin"}
    sort_order              int NOT NULL DEFAULT 0,
    is_flagged              boolean NOT NULL DEFAULT false,
    created_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_arrival_checklist_items_guide
    ON arrival_checklist_items(guide_id, sort_order);

COMMIT;
