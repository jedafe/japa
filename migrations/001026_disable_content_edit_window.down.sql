-- Restore the previous 30-minute author edit window.
UPDATE system_settings
SET value = '"30"'::jsonb, updated_at = now()
WHERE key = 'content.edit_window_minutes';
