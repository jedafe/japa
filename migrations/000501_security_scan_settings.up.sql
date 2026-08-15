-- 000501_security_scan_settings.up.sql
-- Security scan threshold system_settings (Section 40, Rule 3 compliance).
-- Moves the six hardcoded security scan thresholds into system_settings so
-- they are tenant-configurable without code changes.
-- Ref: sections/40-platform-analytics-reports.md, AGENTS.md Rule 3.

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'security.scan.failed_login_threshold',     '50'),
    ('00000000-0000-0000-0000-000000000001', 'security.scan.sla_breach_threshold',        '5'),
    ('00000000-0000-0000-0000-000000000001', 'security.scan.stale_tokens_threshold',      '100'),
    ('00000000-0000-0000-0000-000000000001', 'security.scan.unverified_users_threshold',  '10'),
    ('00000000-0000-0000-0000-000000000001', 'security.scan.min_password_length',         '8'),
    ('00000000-0000-0000-0000-000000000001', 'security.scan.excessive_api_keys_threshold','20')
ON CONFLICT (tenant_id, key) DO NOTHING;
