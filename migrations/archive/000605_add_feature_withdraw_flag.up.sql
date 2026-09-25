-- 000605_add_feature_withdraw_flag.up.sql
-- Adds the feature.withdraw kill switch. Withdrawal is on by default for the
-- default tenant; operators can disable it from the Control Panel / Admin.
-- Ref: Section 42 Feature Flags, Section 58 Credit Withdrawal.

BEGIN;

INSERT INTO feature_flags (key, tenant_id, enabled, config, updated_by)
VALUES (
    'feature.withdraw',
    '00000000-0000-0000-0000-000000000001',
    true,
    '{"name": "Credit Withdrawals", "description": "Enable user credit withdrawals and payouts"}'::jsonb,
    (SELECT id FROM users WHERE id = 'f233a948-0efa-47d4-83c3-1233fa7f403b')
)
ON CONFLICT (key, tenant_id) DO UPDATE SET
    enabled = true,
    config = EXCLUDED.config,
    updated_by = EXCLUDED.updated_by,
    updated_at = now();

COMMIT;
