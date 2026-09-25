-- 000501_security_scan_settings.down.sql
-- Reverses 000501_security_scan_settings.up.sql.

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'security.scan.failed_login_threshold',
    'security.scan.sla_breach_threshold',
    'security.scan.stale_tokens_threshold',
    'security.scan.unverified_users_threshold',
    'security.scan.min_password_length',
    'security.scan.excessive_api_keys_threshold'
  );
