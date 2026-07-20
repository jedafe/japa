-- 000148_add_dm_messages_soft_delete_columns.down.sql

BEGIN;

DROP INDEX IF EXISTS idx_dm_messages_deleted;
ALTER TABLE dm_messages DROP COLUMN IF EXISTS edited_at;
ALTER TABLE dm_messages DROP COLUMN IF EXISTS deleted_at;

COMMIT;
