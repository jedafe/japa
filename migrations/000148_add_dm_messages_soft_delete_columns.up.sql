-- 000148_add_dm_messages_soft_delete_columns.up.sql
-- AUDIT-BE-033: The dm_messages table was created in 000015 without deleted_at or
-- edited_at columns, but application code (dm.sql queries, moderation auto-hide worker)
-- references them. This adds both columns and backfills existing rows.

BEGIN;

ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS edited_at timestamptz;

CREATE INDEX IF NOT EXISTS idx_dm_messages_deleted ON dm_messages (deleted_at);

COMMIT;
