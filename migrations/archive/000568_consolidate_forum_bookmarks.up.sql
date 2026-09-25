-- 000568_consolidate_forum_bookmarks.up.sql
-- Backfill legacy forum_bookmarks rows into the unified bookmarks table,
-- normalising entity_type to forum_thread/forum_reply. Existing rows that
-- conflict on (user_id, entity_type, entity_id) are skipped.
--
-- Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Bookmarks)

INSERT INTO bookmarks (tenant_id, user_id, entity_type, entity_id, folder, folder_id, created_at)
SELECT
    tenant_id,
    user_id,
    CASE WHEN entity_type = 'thread' THEN 'forum_thread' ELSE 'forum_reply' END,
    entity_id,
    COALESCE(folder, 'discussions'),
    NULL,
    created_at
FROM forum_bookmarks
ON CONFLICT (user_id, entity_type, entity_id) DO NOTHING;
