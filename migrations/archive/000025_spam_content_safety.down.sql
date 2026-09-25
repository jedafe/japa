-- Down migration for 000025_spam_content_safety.sql
-- Drops censored words, user blocks, and IP blocks tables, and removes content_hash column from contents.
ALTER TABLE contents DROP COLUMN IF EXISTS content_hash;
DROP TABLE IF EXISTS censored_words;
DROP TABLE IF EXISTS user_blocks;
DROP TABLE IF EXISTS ip_blocks;
