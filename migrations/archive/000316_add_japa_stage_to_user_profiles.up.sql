-- 000316_add_japa_stage_to_user_profiles.up.sql
-- Japa Stage Progress Profiles (§87, ADR-095) — adds japa_stage columns
-- to user_profiles.
-- Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md

BEGIN;

ALTER TABLE user_profiles
    ADD COLUMN IF NOT EXISTS japa_stage              text
        CHECK (japa_stage IN ('considering','planning','preparing','applied','approved','relocated','settled')),
    ADD COLUMN IF NOT EXISTS japa_stage_dest_space_id uuid REFERENCES spaces(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS japa_stage_updated_at    timestamptz,
    ADD COLUMN IF NOT EXISTS japa_stage_is_public     boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS japa_stage_history       jsonb NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS idx_user_profiles_japa_stage
    ON user_profiles(tenant_id, japa_stage)
    WHERE japa_stage IS NOT NULL AND japa_stage_is_public = true;

COMMIT;
