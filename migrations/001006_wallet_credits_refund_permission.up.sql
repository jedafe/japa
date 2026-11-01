-- 001006 — dedicated permission node for credits refunds.
-- wallet.admin.adjust covered both reputation corrections and monetary
-- refunds; credits refunds get their own node so the two can be scoped
-- independently. Granted to whatever roles already hold wallet.admin.adjust.
BEGIN;

INSERT INTO permissions (id, key, description)
VALUES (gen_random_uuid(), 'wallet.admin.credits_refund', 'Admin credits refund')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT rp.role_id, p.id
FROM role_permissions rp
JOIN permissions pa ON pa.id = rp.permission_id AND pa.key = 'wallet.admin.adjust'
CROSS JOIN permissions p
WHERE p.key = 'wallet.admin.credits_refund'
ON CONFLICT DO NOTHING;

COMMIT;
