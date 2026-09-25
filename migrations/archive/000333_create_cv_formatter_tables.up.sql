-- 000333_create_cv_formatter_tables.up.sql
-- International CV / Resume Formatter (§95, ADR-103)
-- — cv_standards + cv_format_sessions.
--
-- Adaptation: §95 references geo_countries(id). Per ADR-053, countries are
-- spaces with level='country'.
--
-- Ref: japaconnect-engineering/sections/95-cv-formatter.md

BEGIN;

CREATE TABLE IF NOT EXISTS cv_standards (
    id                          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    destination_space_id        uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    recommended_length_pages    text,
    photo_policy                text, -- required|optional|not_recommended|prohibited
    date_format                 text,
    recommended_sections        text[],
    tone_guidance               text,
    common_pitfalls             text,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (destination_space_id)
);

CREATE TABLE IF NOT EXISTS cv_format_sessions (
    id                          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id                     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    origin_space_id             uuid REFERENCES spaces(id) ON DELETE SET NULL,
    destination_space_id        uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    input_text                  text NOT NULL,
    output_text                 text,
    output_format               text NOT NULL DEFAULT 'docx', -- docx|pdf
    status                      text NOT NULL DEFAULT 'pending', -- pending|complete|failed
    ai_model                    text,
    tokens_used                 int,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    expires_at                  timestamptz NOT NULL DEFAULT (now() + interval '24 hours')
);

CREATE INDEX IF NOT EXISTS idx_cv_format_sessions_user
    ON cv_format_sessions(user_id, created_at);

COMMIT;
