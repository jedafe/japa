-- 000708_create_pof_calculator_tables.up.sql
-- Proof of Funds (POF) Calculator (§108, ADR-119)
-- — pof_requirements.
--
-- Conventions: post-000668 schema is de-tenanted (no tenant_id); countries are
-- level='country', uuid primary keys.
--
-- Ref: japaconnect-engineering/sections/108-pof-calculator.md

BEGIN;

CREATE TABLE IF NOT EXISTS pof_requirements (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    destination_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    visa_type               text NOT NULL,   -- express_entry|study_permit|work_visa|family|visitor|job_seeker|other
    program_label           text NOT NULL,   -- e.g. "Express Entry (FSW/FST)", "UK Student Visa (outside London)"
    amount_schedule         jsonb NOT NULL,  -- [{"members":1,"amount":15263},...] OR {"base":N,"base_members":K,"per_additional":M}
    currency                char(3) NOT NULL,
    funds_must_be_liquid    boolean NOT NULL DEFAULT true,
    accepted_proof          jsonb,           -- ["bank_statements","fixed_deposits","blocked_account","scholarship_letter",...]
    notes                   text,
    effective_date          date NOT NULL,
    source_url              text,
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
    UNIQUE (destination_space_id, visa_type, program_label)
);

CREATE INDEX IF NOT EXISTS idx_pof_requirements_dest
    ON pof_requirements(destination_space_id, is_active);

COMMIT;
