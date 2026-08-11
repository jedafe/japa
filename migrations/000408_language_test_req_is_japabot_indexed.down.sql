-- 000408_language_test_req_is_japabot_indexed.down.sql
-- Reverse of 000408: drop the is_japabot_indexed generated column.

BEGIN;

ALTER TABLE language_test_requirements DROP COLUMN IF EXISTS is_japabot_indexed;

COMMIT;
