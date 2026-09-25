-- Down migration for 000103_content_tags_shares_bookmarks_comments.sql
-- Drops comment_votes, comments, bookmarks, content_shares, and content_tags tables.
DROP TABLE IF EXISTS comment_votes;
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS bookmarks;
DROP TABLE IF EXISTS content_shares;
DROP TABLE IF EXISTS content_tags;
