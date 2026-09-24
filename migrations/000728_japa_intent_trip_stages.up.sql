-- 000728_japa_intent_trip_stages.up.sql
-- ADR-122 amendment: visit/business intents carry stages. Widen the stage
-- vocabulary to nine values (adds traveled, returned) and replace the
-- intent×stage CHECK with the class-scoped rule — each journey class owns
-- its terminal pair (relocated/settled vs traveled/returned).
-- Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
--      japaconnect-engineering/adr/ADR-122-japa-intent.md

BEGIN;

ALTER TABLE user_profiles
    DROP CONSTRAINT IF EXISTS user_profiles_japa_stage_check,
    DROP CONSTRAINT IF EXISTS user_profiles_japa_stage_chk,
    DROP CONSTRAINT IF EXISTS user_profiles_intent_stage_chk;

ALTER TABLE user_profiles
    ADD CONSTRAINT user_profiles_japa_stage_chk
        CHECK (japa_stage IN ('considering','planning','preparing','applied','approved',
                              'relocated','settled','traveled','returned')),
    ADD CONSTRAINT user_profiles_intent_stage_chk
        CHECK (
            japa_stage IS NULL
            OR (
                (japa_intent IS NULL OR japa_intent IN ('relocation','study','work','family'))
                AND japa_stage IN ('considering','planning','preparing','applied','approved',
                                   'relocated','settled')
            )
            OR (
                japa_intent IN ('visit','business')
                AND japa_stage IN ('considering','planning','preparing','applied','approved',
                                   'traveled','returned')
            )
        );

COMMIT;
