-- 000567_drop_bookmarks_entity_type_check.up.sql
-- Remove the restrictive entity_type CHECK on bookmarks so the table can
-- hold bookmarks for any content or entity type (doc_page, forum_reply,
-- future types). The previous constraint (000505) only covered a fixed list
-- and rejected valid entity types such as doc_page and forum_reply.
--
-- Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md (bookmarks)

ALTER TABLE bookmarks DROP CONSTRAINT IF EXISTS bookmarks_entity_type_check;
