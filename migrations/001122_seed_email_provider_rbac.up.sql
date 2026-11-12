-- 001122 — seed email.providers.* RBAC nodes for the Control Panel
-- /control/email-providers routes (ADR-015 multi-provider management).
--
-- Nodes:
--   email.providers.manage — create, update, delete, test email providers
--   email.providers.view   — view email provider list and config (read-only)
--
-- Seeded completely (BE-005): both nodes granted to super_admin.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('email.providers.manage', 'Create, update, delete, and test email provider configurations (API keys, SMTP hosts, category routing)'),
    ('email.providers.view',   'View email provider list and configuration (read-only)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE r.slug = 'super_admin'
  AND p.key IN ('email.providers.manage', 'email.providers.view')
ON CONFLICT DO NOTHING;

COMMIT;
