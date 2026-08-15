-- Reverse: remove the users.username_cooldown_days system setting.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'users.username_cooldown_days';
