-- 000206_users_username_changed_at.up.sql
-- Add username_changed_at to track the 30-day username change cooldown
-- required by Section 50 (PATCH /me/account). NULL means never changed.
-- Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md

ALTER TABLE users ADD COLUMN IF NOT EXISTS username_changed_at timestamptz;

CREATE INDEX IF NOT EXISTS idx_users_username_changed_at ON users(username_changed_at);
