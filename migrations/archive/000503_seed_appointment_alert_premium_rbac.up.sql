-- 000503_seed_appointment_alert_premium_rbac.up.sql
-- RBAC node for premium-tier embassy appointment alerter features (§91, ADR-099).
--   appointment_alert.premium — premium+
-- Premium users get 5-minute polling (vs 15-minute standard) and access to the
-- SMS notification channel. The node is granted to the 'premium' role (and
-- admin/super_admin for completeness). Non-premium users who select the SMS
-- channel are rejected by the service layer with a 403 sms_premium_only error.
--
-- Ref: japaconnect-engineering/sections/91-embassy-appointment-alerter.md
--      japaconnect-engineering/adr/ADR-099-embassy-appointment-alerter.md

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('appointment_alert.premium', 'Premium-tier appointment alerter features: 5-min polling + SMS channel (premium+)')
ON CONFLICT (key) DO NOTHING;

-- Grant to premium, admin, and super_admin roles. The 'premium' role is the
-- standard premium-tier role in JapaConnect (see 000398_fix_cv_formatter_rbac_grants).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('premium', 'admin', 'super_admin')
  AND p.key = 'appointment_alert.premium'
ON CONFLICT DO NOTHING;

COMMIT;
