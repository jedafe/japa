-- 000206_users_username_changed_at.down.sql
-- Reverse of 000206: drop the username_changed_at column.

DROP INDEX IF EXISTS idx_users_username_changed_at;
ALTER TABLE users DROP COLUMN IF EXISTS username_changed_at;
