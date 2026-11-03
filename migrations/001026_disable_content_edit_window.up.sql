-- Disable the author edit window: 0 means members may edit their own
-- published posts at any time. The 30-minute limit (after the stale-write
-- fix surfaced it as the real blocker) made own-post edits impossible
-- hours after publishing. Comments/forum windows are separate keys and
-- unaffected.
UPDATE system_settings
SET value = '"0"'::jsonb, updated_at = now()
WHERE key = 'content.edit_window_minutes';
