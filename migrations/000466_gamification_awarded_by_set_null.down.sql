-- 000466_gamification_awarded_by_set_null.down.sql
-- Revert the awarded_by FK to the original NO ACTION behavior.
-- Ref: japaconnect-engineering/sections/64-gamification.md

BEGIN;

ALTER TABLE user_badges
    DROP CONSTRAINT IF EXISTS user_badges_awarded_by_fkey;

ALTER TABLE user_badges
    ADD CONSTRAINT user_badges_awarded_by_fkey
    FOREIGN KEY (awarded_by) REFERENCES users(id);

COMMIT;
