-- 000466_gamification_awarded_by_set_null.up.sql
-- Alter user_badges.awarded_by FK to ON DELETE SET NULL (BUG-007, §15/§19).
--
-- The awarded_by column references users(id) without an ON DELETE action,
-- which defaults to NO ACTION. If an admin who has manually awarded badges
-- is hard-deleted (GDPR erasure), the DELETE FROM users fails with an FK
-- violation because the user_badges rows still reference the admin's
-- user_id. Setting ON DELETE SET NULL preserves the badge (the user keeps
-- their earned badge) while removing the admin reference.
--
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
--      japaconnect-engineering/sections/64-gamification.md

BEGIN;

-- Drop the existing FK constraint and recreate it with ON DELETE SET NULL.
-- The constraint name follows the default PostgreSQL naming convention:
-- <table>_<column>_fkey.
ALTER TABLE user_badges
    DROP CONSTRAINT IF EXISTS user_badges_awarded_by_fkey;

ALTER TABLE user_badges
    ADD CONSTRAINT user_badges_awarded_by_fkey
    FOREIGN KEY (awarded_by) REFERENCES users(id) ON DELETE SET NULL;

COMMIT;
