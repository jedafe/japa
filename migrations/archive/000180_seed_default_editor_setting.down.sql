-- Reverse: remove the appearance.default_editor setting seeded by the up migration.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'appearance.default_editor';
