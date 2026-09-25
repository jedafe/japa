-- 000709_create_eca_guide_tables.up.sql
-- Educational Credential Assessment (ECA) Guide (§109, ADR-120)
-- — eca_providers + eca_guides + eca_community_updates.
--
-- Conventions: post-000668 schema is de-tenanted (no tenant_id); countries are
-- level='country', uuid primary keys.
--
-- Ref: japaconnect-engineering/sections/109-eca-guide.md

BEGIN;

CREATE TABLE IF NOT EXISTS eca_providers (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       text NOT NULL UNIQUE,   -- wes|iqas|icas|ces|ecctis|icewg|other
    name       text NOT NULL,
    website    text,
    is_active  boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS eca_guides (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    destination_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    provider_id             uuid NOT NULL REFERENCES eca_providers(id) ON DELETE RESTRICT,
    purpose                 text NOT NULL,   -- express_entry|study_admission|employment|licensing_support
    pathway_steps           jsonb NOT NULL,  -- [{step, action, detail, fee_estimate, turnaround, official_url}]
    fee_estimate            text,
    currency                char(3),
    timeline_estimate       text,
    documents_required      jsonb,           -- [{document, notes}]
    accepted_for            jsonb,           -- programs/purposes this provider is accepted for at this destination
    official_url            text,
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
    UNIQUE (destination_space_id, provider_id, purpose)
);

CREATE INDEX IF NOT EXISTS idx_eca_guides_dest
    ON eca_guides(destination_space_id, is_active);
CREATE INDEX IF NOT EXISTS idx_eca_guides_provider
    ON eca_guides(provider_id, is_active);

CREATE TABLE IF NOT EXISTS eca_community_updates (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    guide_id        uuid NOT NULL REFERENCES eca_guides(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    step_index      int,
    fee_estimate    text,
    turnaround_days int,
    note            text,
    status          text NOT NULL DEFAULT 'pending',  -- pending|approved|rejected
    reviewed_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz       -- Rule 11: soft-delete user content
);

CREATE INDEX IF NOT EXISTS idx_eca_updates_guide
    ON eca_community_updates(guide_id, status, created_at)
    WHERE deleted_at IS NULL;

COMMIT;
