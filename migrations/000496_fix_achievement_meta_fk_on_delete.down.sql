-- 000496_fix_achievement_meta_fk_on_delete.down.sql
-- Reverts BUG-002 fix: restores the original ON DELETE SET NULL FK from
-- migration 000320 (which was contradictory with the NOT NULL constraint
-- added in 000442). This down migration exists for completeness only;
-- rolling back would re-introduce the contradiction.
--
-- Ref: japaconnect-engineering/sections/89-achievement-feed.md

BEGIN;

ALTER TABLE achievement_meta
    DROP CONSTRAINT IF EXISTS achievement_meta_destination_space_id_fkey;

ALTER TABLE achievement_meta
    ADD CONSTRAINT achievement_meta_destination_space_id_fkey
    FOREIGN KEY (destination_space_id) REFERENCES spaces(id) ON DELETE SET NULL;

COMMIT;
