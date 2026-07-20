-- Restore feed.card_style to its scalar form (the default value).
UPDATE system_settings
SET value = value->'default',
    updated_at = now()
WHERE key = 'feed.card_style'
  AND jsonb_typeof(value) = 'object'
  AND value ? 'default';
