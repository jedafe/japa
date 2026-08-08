-- 000331_create_credential_checker_tables.up.sql
-- Professional Credential Recognition Checker (§94, ADR-102)
-- — credential_professions + credential_pathways + credential_community_notes.
--
-- Adaptation: §94 references sites(id), geo_countries(id). Per ADR-053,
-- tenancy uses tenants(id) (uuid), countries are spaces with level='country'.
--
-- Ref: japaconnect-engineering/sections/94-professional-credential-checker.md

BEGIN;

CREATE TABLE IF NOT EXISTS credential_professions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        text NOT NULL UNIQUE,
    name        text NOT NULL,
    category    text, -- medical|engineering|legal|finance|education|other
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS credential_pathways (
    id                          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    profession_id               uuid NOT NULL REFERENCES credential_professions(id) ON DELETE RESTRICT,
    issuing_space_id            uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    destination_space_id        uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    regulatory_body             text NOT NULL,
    regulatory_body_url         text,
    pathway_type                text NOT NULL, -- direct|bridging_exam|supervised_practice|full_requalification
    timeline_estimate           text,
    cost_estimate_usd           text,
    pathway_description         text NOT NULL,
    official_application_url    text,
    source_url_last_checked     date,
    status                      text NOT NULL DEFAULT 'published', -- draft|ai_draft|pending|published|rejected
    is_active                   boolean NOT NULL DEFAULT true,
    ai_generated                boolean NOT NULL DEFAULT false,
    ai_model                    text,
    ai_prompt_hash              text,
    hitl_reviewed_by            uuid REFERENCES users(id) ON DELETE SET NULL,
    hitl_reviewed_at            timestamptz,
    hitl_review_notes           text,
    is_japabot_indexed          boolean NOT NULL DEFAULT false,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, profession_id, issuing_space_id, destination_space_id)
);

CREATE INDEX IF NOT EXISTS idx_credential_pathways_tenant_dest
    ON credential_pathways(tenant_id, destination_space_id, is_active);
CREATE INDEX IF NOT EXISTS idx_credential_pathways_tenant_prof
    ON credential_pathways(tenant_id, profession_id, is_active);

CREATE TABLE IF NOT EXISTS credential_community_notes (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pathway_id      uuid NOT NULL REFERENCES credential_pathways(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note            text NOT NULL,
    upvotes         int NOT NULL DEFAULT 0,
    status          text NOT NULL DEFAULT 'published',
    created_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz -- Rule 11: soft-delete user content
);

CREATE INDEX IF NOT EXISTS idx_credential_notes_pathway
    ON credential_community_notes(pathway_id, status, created_at);

COMMIT;
