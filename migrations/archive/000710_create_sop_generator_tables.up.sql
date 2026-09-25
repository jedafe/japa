-- 000710_create_sop_generator_tables.up.sql
-- AI SOP Generator (§110, ADR-121)
-- — sop_templates + sop_samples + sop_drafts.
--
-- Conventions: post-000668 schema is de-tenanted (no tenant_id); countries are
-- level='country', uuid primary keys.
--
-- Ref: japaconnect-engineering/sections/110-sop-generator.md

BEGIN;

CREATE TABLE IF NOT EXISTS sop_templates (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    destination_space_id uuid REFERENCES spaces(id) ON DELETE RESTRICT,  -- NULL = generic
    purpose              text NOT NULL,   -- study_permit|student_visa|work_visa|visitor_visa
    title                text NOT NULL,
    structure            jsonb NOT NULL,  -- [{section, prompt_hint, max_words}]
    status               text NOT NULL DEFAULT 'published',
    is_active            boolean NOT NULL DEFAULT true,
    ai_generated         boolean NOT NULL DEFAULT false,
    ai_model             text,
    ai_prompt_hash       text,
    hitl_reviewed_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    hitl_reviewed_at     timestamptz,
    hitl_review_notes    text,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (destination_space_id, purpose, title)
);

CREATE TABLE IF NOT EXISTS sop_samples (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    destination_space_id uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    program_level        text NOT NULL,   -- diploma|bachelors|masters|mba|phd
    field_of_study       text,
    title                text NOT NULL,
    body                 text NOT NULL,
    word_count           int,
    status               text NOT NULL DEFAULT 'draft', -- draft|ai_draft|pending|published|rejected
    ai_generated         boolean NOT NULL DEFAULT false,
    ai_model             text,
    ai_prompt_hash       text,
    hitl_reviewed_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    hitl_reviewed_at     timestamptz,
    hitl_review_notes    text,
    is_japabot_indexed   boolean NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sop_samples_dest
    ON sop_samples(destination_space_id, status);
CREATE INDEX IF NOT EXISTS idx_sop_templates_dest
    ON sop_templates(destination_space_id, is_active);

CREATE TABLE IF NOT EXISTS sop_drafts (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    destination_space_id uuid REFERENCES spaces(id) ON DELETE SET NULL,
    purpose              text NOT NULL,
    program              text,
    institution          text,
    title                text,
    inputs               jsonb NOT NULL,      -- profile answers used for generation
    generated_text       text,                -- NULL until sop:generate completes
    status               text NOT NULL DEFAULT 'queued', -- queued|generated|failed
    ai_generated         boolean NOT NULL DEFAULT true,
    ai_model             text,
    ai_prompt_hash       text,
    version              int NOT NULL DEFAULT 1,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    deleted_at           timestamptz          -- Rule 11: soft-delete user content
);

CREATE INDEX IF NOT EXISTS idx_sop_drafts_user
    ON sop_drafts(user_id, created_at DESC)
    WHERE deleted_at IS NULL;

COMMIT;
