-- 000568_consolidate_forum_bookmarks.down.sql
-- Remove rows that were backfilled from forum_bookmarks. This only deletes
-- rows that have a matching (user_id, entity_id) pair in the legacy table
-- and an entity_type of forum_thread/forum_reply.

DELETE FROM bookmarks
WHERE entity_type IN ('forum_thread', 'forum_reply')
  AND (user_id, entity_id) IN (
      SELECT user_id, entity_id FROM forum_bookmarks
  );
