-- 000337_create_attestation_guide_tables.up.sql
-- Document Attestation & Apostille Guide (§97, ADR-105)
-- — attestation_pathways + attestation_community_updates.
--
-- Adaptation: §97 references sites(id), geo_countries(id). Per ADR-053,
-- tenancy uses tenants(id) (uuid), countries are spaces with level='country'.
--
-- Ref: japaconnect-engineering/sections/97-attestation-guide.md

BEGIN;

CREATE TABLE IF NOT EXISTS attestation_pathways (
    id                          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    document_type               text NOT NULL, -- birth_certificate|degree|marriage|police_clearance|professional_licence|other
    issuing_space_id            uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    destination_space_id        uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    apostille_required          boolean NOT NULL,
    pathway_steps               jsonb NOT NULL, -- [{step, agency, what_to_bring, fee_estimate, turnaround_days, official_url}]
    community_last_verified     date,
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
    UNIQUE (tenant_id, document_type, issuing_space_id, destination_space_id)
);

CREATE INDEX IF NOT EXISTS idx_attestation_pathways_tenant_dest
    ON attestation_pathways(tenant_id, destination_space_id, is_active);
CREATE INDEX IF NOT EXISTS idx_attestation_pathways_tenant_doc
    ON attestation_pathways(tenant_id, document_type, is_active);

CREATE TABLE IF NOT EXISTS attestation_community_updates (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pathway_id          uuid NOT NULL REFERENCES attestation_pathways(id) ON DELETE CASCADE,
    user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    step_index          int,
    fee_estimate        text,
    turnaround_days     int,
    note                text,
    status              text NOT NULL DEFAULT 'pending', -- pending|approved|rejected
    reviewed_by         uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz -- Rule 11: soft-delete user content
);

CREATE INDEX IF NOT EXISTS idx_attestation_updates_pathway
    ON attestation_community_updates(pathway_id, status);

COMMIT;
