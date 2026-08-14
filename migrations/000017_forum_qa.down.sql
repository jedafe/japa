-- Down migration for 000017_forum_qa.sql
-- Drops forum Q&A tables, trigger, and function; removes parent_reply_id column/constraint from forum_threads.
DROP TRIGGER IF EXISTS trg_reply_views_increment ON reply_views;
DROP FUNCTION IF EXISTS increment_reply_view_count();
DROP TABLE IF EXISTS reply_views;
DROP TABLE IF EXISTS forum_bookmarks;
DROP TABLE IF EXISTS forum_thread_tags;
DROP TABLE IF EXISTS forum_tags;
DROP TABLE IF EXISTS forum_poll_votes;
DROP TABLE IF EXISTS forum_poll_options;
DROP TABLE IF EXISTS forum_polls;
DROP TABLE IF EXISTS forum_votes;
ALTER TABLE forum_threads DROP CONSTRAINT IF EXISTS fk_forum_threads_parent_reply;
ALTER TABLE forum_threads DROP COLUMN IF EXISTS parent_reply_id;
DROP TABLE IF EXISTS forum_replies;
DROP TABLE IF EXISTS forum_threads;
