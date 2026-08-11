-- Reverse: remove the three tag custom fields and revert content.max_tags publicity.
DELETE FROM custom_fields
WHERE id IN (
    '00000000-0000-0000-0000-000000000053',
    '00000000-0000-0000-0000-000000000054',
    '00000000-0000-0000-0000-000000000055'
);

UPDATE system_settings
SET is_public = false
WHERE key = 'content.max_tags';
