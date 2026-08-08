-- 000346_users_password_changed_at.up.sql
-- Add password_changed_at column to users table.
-- Ref: sections/02-authentication-sessions.md — "Sets users.password_changed_at = now()"
-- on every password change/reset. Used to enforce max password age policies and
-- to audit when a user last rotated their credentials.
--
-- Zero-downtime: nullable column with no default, additive only.
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_changed_at timestamptz;
