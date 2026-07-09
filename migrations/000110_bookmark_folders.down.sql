-- Reverts 000110_bookmark_folders.
BEGIN;
DROP TABLE IF EXISTS bookmark_folders;
COMMIT;
