-- 000335_create_housing_guide_tables.up.sql
-- Housing & Accommodation Finder Guide (§96, ADR-104) — housing_guides.
--
-- Adaptation: §96 references sites(id), geo_countries(id), geo_cities(id).
-- Per ADR-053, tenancy uses tenants(id) (uuid), countries and cities are
-- spaces with level='country' and level='city' respectively.
--
-- Ref: japaconnect-engineering/sections/96-housing-guide.md

BEGIN;

CREATE TABLE IF NOT EXISTS housing_guides (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    destination_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    city_space_id           uuid REFERENCES spaces(id) ON DELETE SET NULL, -- NULL = country-level
    accommodation_type      text, -- private_rental|flatshare|student|employer_provided|all
    search_platforms        jsonb, -- [{name, url, description}]
    landlord_requirements   text,
    rent_ranges             jsonb, -- {studio, 1bed, 2bed} × {city_centre, inner, outer}
    scam_categories         text[],
    tips                    text,
    status                  text NOT NULL DEFAULT 'published', -- draft|ai_draft|pending|published|rejected
    is_active               boolean NOT NULL DEFAULT true,
    ai_generated            boolean NOT NULL DEFAULT false,
    ai_model                text,
    ai_prompt_hash          text,
    hitl_reviewed_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    hitl_reviewed_at        timestamptz,
    hitl_review_notes       text,
    last_verified_at        date,
    is_japabot_indexed      boolean NOT NULL DEFAULT false,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, destination_space_id, city_space_id, accommodation_type)
);

CREATE INDEX IF NOT EXISTS idx_housing_guides_tenant_dest
    ON housing_guides(tenant_id, destination_space_id, is_active);

COMMIT;
