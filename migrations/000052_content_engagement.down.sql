-- Down migration for 000052_content_engagement.sql
-- Drops the content_engagement table used for velocity scoring.
DROP TABLE IF EXISTS content_engagement;
