-- Reshape feed.card_style from a scalar string to a {default, by_type} object.
-- Backward-compatible: the Go reader (resolveCardStyleConfig) also tolerates
-- the legacy scalar, so this is data hygiene, not a hard requirement.
-- Guarded by jsonb_typeof and therefore idempotent.
UPDATE system_settings
SET value = jsonb_build_object('default', value, 'by_type', '{}'::jsonb),
    updated_at = now()
WHERE key = 'feed.card_style'
  AND jsonb_typeof(value) = 'string';
