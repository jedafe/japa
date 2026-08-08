-- 000346_users_password_changed_at.down.sql
-- Reverses 000346: drops the password_changed_at column from users.
ALTER TABLE users DROP COLUMN IF EXISTS password_changed_at;
