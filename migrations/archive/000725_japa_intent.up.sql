-- 000725_japa_intent.up.sql
-- Japa Intent (§87, ADR-122) — adds the journey-class column to
-- user_profiles beside the ADR-095 stage machine.
-- Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
--      japaconnect-engineering/adr/ADR-122-japa-intent.md

BEGIN;

ALTER TABLE user_profiles
    ADD COLUMN IF NOT EXISTS japa_intent text;

ALTER TABLE user_profiles
    ADD CONSTRAINT user_profiles_japa_intent_chk
        CHECK (japa_intent IN ('relocation','study','work','family','visit','business')),
    ADD CONSTRAINT user_profiles_intent_stage_chk
        CHECK (japa_intent IN ('relocation','study','work','family') OR japa_stage IS NULL);

-- Members who already declared a relocation stage get the matching intent.
-- Rows that never engaged the feature stay NULL (undeclared).
UPDATE user_profiles
SET japa_intent = 'relocation'
WHERE japa_intent IS NULL AND japa_stage IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_user_profiles_japa_intent
    ON user_profiles(japa_intent)
    WHERE japa_intent IS NOT NULL AND japa_stage_is_public = true;

COMMIT;
