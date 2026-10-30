BEGIN;

ALTER TABLE comment_votes DROP COLUMN IF EXISTS updated_at;
ALTER TABLE forum_votes DROP COLUMN IF EXISTS updated_at;

COMMIT;
