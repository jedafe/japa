-- Reverse: drop the session tracking columns added in 000356.
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS last_used_at;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS ip_address;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS device_name;
