-- 000339_create_language_test_hub_tables.up.sql
-- Language Test Preparation Hub (§99, ADR-107)
-- — language_tests + language_test_requirements + language_test_resources + language_score_stories.
--
-- Adaptation: §99 references sites(id), geo_countries(id), visa_categories(id).
-- Per ADR-053, tenancy uses tenants(id) (uuid), countries are spaces with
-- level='country', and visa category is stored as nullable text.
--
-- Ref: japaconnect-engineering/sections/99-language-test-hub.md

BEGIN;

CREATE TABLE IF NOT EXISTS language_tests (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            text NOT NULL UNIQUE, -- ielts|oet|toefl|pte|celpip|goethe|delf|other
    name            text NOT NULL,
    official_url    text,
    validity_months int,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS language_test_requirements (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    destination_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    visa_category           text, -- NULL = general requirement
    test_id                 uuid NOT NULL REFERENCES language_tests(id) ON DELETE RESTRICT,
    minimum_score           text NOT NULL,
    score_breakdown         jsonb,
    notes                   text,
    source_url              text,
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
    UNIQUE (tenant_id, destination_space_id, visa_category, test_id)
);

CREATE INDEX IF NOT EXISTS idx_language_test_req_tenant_dest
    ON language_test_requirements(tenant_id, destination_space_id, is_active);

CREATE TABLE IF NOT EXISTS language_test_resources (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    test_id             uuid NOT NULL REFERENCES language_tests(id) ON DELETE CASCADE,
    title               text NOT NULL,
    resource_url        text NOT NULL,
    resource_type       text, -- free|paid|official|community
    community_rating    numeric(3,2),
    rating_count        int NOT NULL DEFAULT 0,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_language_test_resources_test
    ON language_test_resources(tenant_id, test_id);

CREATE TABLE IF NOT EXISTS language_score_stories (
    id                          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id                     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    test_id                     uuid NOT NULL REFERENCES language_tests(id) ON DELETE RESTRICT,
    starting_level              text,
    final_score                 text NOT NULL,
    destination_space_id        uuid REFERENCES spaces(id) ON DELETE SET NULL,
    prep_duration_weeks         int,
    resources_used              text[],
    advice                      text,
    status                      text NOT NULL DEFAULT 'published',
    created_at                  timestamptz NOT NULL DEFAULT now(),
    deleted_at                  timestamptz -- Rule 11: soft-delete user content
);

CREATE INDEX IF NOT EXISTS idx_language_score_stories_test
    ON language_score_stories(tenant_id, test_id, status);

COMMIT;
