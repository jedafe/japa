-- 000505_bookmarks_entity_type_check_expand.up.sql
-- Fix BUG-001: The `bookmarks` table (000103) was created with
-- `CHECK (entity_type IN ('content', 'thread', 'reply'))`, but §08 spec
-- defines a single polymorphic bookmarks table that works across content,
-- space, tag, comment (and the platform has since added blog_post,
-- forum_thread, learning_track entity types). The /me/bookmarks query
-- (users.sql ListUserBookmarks) joins on forum_thread/space/tag/blog_post,
-- the learn app requests entity_type=learning_track, and the participation
-- dispatcher (server.go) can call CreateBookmark with arbitrary
-- TargetEntityType. Any insert with entity_type outside ('content',
-- 'thread', 'reply') was rejected by the CHECK constraint, silently
-- breaking space/tag/blog_post/forum_thread/learning_track bookmarks.
--
-- This migration drops the restrictive CHECK and replaces it with one
-- covering all entity types the platform actually bookmarks. The list is
-- derived from the /me/bookmarks query CASE branches + the learn app's
-- learning_track + the §08 spec's content/space/tag/comment set.
-- Zero-downtime: ALTER CONSTRAINT is a catalog-only change (no table
-- rewrite, no lock beyond a brief AccessExclusive to swap the constraint).

BEGIN;

ALTER TABLE bookmarks DROP CONSTRAINT IF EXISTS bookmarks_entity_type_check;

ALTER TABLE bookmarks ADD CONSTRAINT bookmarks_entity_type_check
    CHECK (entity_type IN (
        'content', 'blog_post', 'forum_thread', 'thread', 'reply',
        'space', 'tag', 'comment', 'learning_track'
    ));

COMMIT;
