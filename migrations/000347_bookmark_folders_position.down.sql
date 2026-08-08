-- Reverts 000347_bookmark_folders_position.
BEGIN;

ALTER TABLE bookmark_folders DROP COLUMN IF EXISTS position;

COMMIT;
