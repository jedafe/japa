-- Add device_name, ip_address, last_used_at to refresh_tokens so the
-- /me/sessions endpoint can surface the contract defined in JEOS §50
-- (id, device_name, ip_address, last_used_at, created_at). All nullable
-- because existing rows predate the columns.
-- Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md

ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS device_name text;
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS ip_address text;
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS last_used_at timestamptz;
