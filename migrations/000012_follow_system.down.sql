-- Down migration for 000012_follow_system.sql
-- Drops follows and tags tables, removes follower_count column from users.
DROP TABLE IF EXISTS follows;
DROP TABLE IF EXISTS tags;
ALTER TABLE users DROP COLUMN IF EXISTS follower_count;
