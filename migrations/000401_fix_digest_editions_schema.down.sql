-- 000401_fix_digest_editions_schema.down.sql
-- Revert the schema changes from 000401.

BEGIN;

ALTER TABLE digest_editions
    ALTER COLUMN generated_by DROP NOT NULL,
    ALTER COLUMN generated_by DROP DEFAULT;

COMMENT ON COLUMN digest_editions.status IS 'draft|scheduled|published';

COMMIT;
