-- 000496_fix_achievement_meta_fk_on_delete.up.sql
-- Fixes BUG-002: achievement_meta.destination_space_id is NOT NULL (set in
-- migration 000442) but the FK has ON DELETE SET NULL (from migration 000320).
-- This is contradictory: if a country space with achievements is deleted,
-- PostgreSQL tries to SET NULL on a NOT NULL column → the DELETE fails with
-- a constraint violation. The spec (§89) has no ON DELETE clause (default
-- NO ACTION), meaning the intent is to block deletion of a country that has
-- achievements. This migration changes the FK to ON DELETE NO ACTION to
-- match the spec intent and remove the contradiction.
--
-- Ref: japaconnect-engineering/sections/89-achievement-feed.md
--      .jeos/audit/achievement-audit.md BUG-002

BEGIN;

-- Drop the contradictory FK and recreate with ON DELETE NO ACTION.
-- The constraint name is the auto-generated form from migration 000320.
ALTER TABLE achievement_meta
    DROP CONSTRAINT IF EXISTS achievement_meta_destination_space_id_fkey;

ALTER TABLE achievement_meta
    ADD CONSTRAINT achievement_meta_destination_space_id_fkey
    FOREIGN KEY (destination_space_id) REFERENCES spaces(id) ON DELETE NO ACTION;

COMMIT;
