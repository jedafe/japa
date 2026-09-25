-- 000707_create_crs_calculator_tables.up.sql
-- Express Entry CRS Calculator (§107, ADR-118)
-- — crs_rulesets + crs_factor_points + crs_draws + crs_score_history.
--
-- Conventions: post-000668 schema is de-tenanted (no tenant_id); countries are
-- level='country', uuid primary keys.
--
-- Ref: japaconnect-engineering/sections/107-crs-calculator.md

BEGIN;

CREATE TABLE IF NOT EXISTS crs_rulesets (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version_label           text NOT NULL,        -- e.g. "IRCC 2025-03 rules"
    effective_date          date NOT NULL,
    status                  text NOT NULL DEFAULT 'draft', -- draft|active|superseded
    source_url              text,
    source_url_last_checked date,
    ai_generated            boolean NOT NULL DEFAULT false,
    ai_model                text,
    ai_prompt_hash          text,
    hitl_reviewed_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    hitl_reviewed_at        timestamptz,
    hitl_review_notes       text,
    is_japabot_indexed      boolean NOT NULL DEFAULT false,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (version_label)
);

CREATE TABLE IF NOT EXISTS crs_factor_points (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ruleset_id         uuid NOT NULL REFERENCES crs_rulesets(id) ON DELETE CASCADE,
    factor_group       text NOT NULL,  -- core|spouse|transferability|additional
    factor_key         text NOT NULL,  -- age|education|language_first|language_second|canadian_experience|foreign_experience|provincial_nomination|job_offer|sibling|french|canadian_education|certificate_of_qualification|spouse_language|spouse_education|spouse_canadian_experience
    input_descriptor   jsonb NOT NULL, -- {"min_age":18,"max_age":29} | {"clb":9} | {"edu":"masters"}
    label              text NOT NULL,
    points_single      int NOT NULL,
    points_with_spouse int,            -- NULL where not applicable
    sort_order         int DEFAULT 0,
    UNIQUE (ruleset_id, factor_group, factor_key, input_descriptor)
);

CREATE INDEX IF NOT EXISTS idx_crs_factor_points_ruleset
    ON crs_factor_points(ruleset_id, factor_group, sort_order);

CREATE TABLE IF NOT EXISTS crs_draws (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    draw_number        int,
    draw_date          date NOT NULL,
    category           text NOT NULL DEFAULT 'general', -- general|cec|pnp|french|stem|health|trades|agriculture|education|transport
    crs_cutoff         int NOT NULL,
    invitations_issued int NOT NULL,
    tie_break_rule     text,
    source_url         text,
    status             text NOT NULL DEFAULT 'published',
    hitl_reviewed_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    hitl_reviewed_at   timestamptz,
    is_japabot_indexed boolean NOT NULL DEFAULT false,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (draw_date, category, draw_number)
);

CREATE INDEX IF NOT EXISTS idx_crs_draws_cat
    ON crs_draws(category, draw_date DESC);

CREATE TABLE IF NOT EXISTS crs_score_history (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ruleset_id  uuid NOT NULL REFERENCES crs_rulesets(id) ON DELETE RESTRICT,
    inputs      jsonb NOT NULL,
    score       int NOT NULL,
    breakdown   jsonb NOT NULL,   -- per-factor awarded points (explainability)
    created_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz       -- Rule 11: soft-delete user content
);

CREATE INDEX IF NOT EXISTS idx_crs_score_history_user
    ON crs_score_history(user_id, created_at DESC)
    WHERE deleted_at IS NULL;

COMMIT;
