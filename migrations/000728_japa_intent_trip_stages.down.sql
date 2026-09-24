-- 000728_japa_intent_trip_stages.down.sql
-- Reverts to the pre-amendment constraints: seven-stage vocabulary and
-- NULL stage for non-relocation intents. Stages that only exist under the
-- new vocabulary (traveled/returned) and any stage on a visit/business
-- profile are cleared first — that data does not survive a rollback.

BEGIN;

ALTER TABLE user_profiles
    DROP CONSTRAINT IF EXISTS user_profiles_japa_stage_chk,
    DROP CONSTRAINT IF EXISTS user_profiles_intent_stage_chk;

UPDATE user_profiles
SET japa_stage = NULL,
    japa_stage_updated_at = now()
WHERE japa_stage IN ('traveled','returned')
   OR (japa_stage IS NOT NULL AND japa_intent IN ('visit','business'));

ALTER TABLE user_profiles
    ADD CONSTRAINT user_profiles_japa_stage_check
        CHECK (japa_stage IN ('considering','planning','preparing','applied','approved',
                              'relocated','settled')),
    ADD CONSTRAINT user_profiles_intent_stage_chk
        CHECK (japa_intent IN ('relocation','study','work','family') OR japa_stage IS NULL);

COMMIT;
