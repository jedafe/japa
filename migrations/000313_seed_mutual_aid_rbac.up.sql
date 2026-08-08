-- 000313_seed_mutual_aid_rbac.up.sql
-- RBAC nodes for Mutual Aid Board (§85, ADR-093).
--   mutual_aid.create_need        — member with reputation ≥ threshold
--   mutual_aid.create_offer       — member+
--   mutual_aid.create_skill_swap  — member+
--   mutual_aid.moderate           — moderator, admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('mutual_aid.create_need',       'Post a need on the mutual aid board (reputation-gated)'),
    ('mutual_aid.create_offer',      'Post an offer on the mutual aid board (member+)'),
    ('mutual_aid.create_skill_swap', 'Post a skill swap on the mutual aid board (member+)'),
    ('mutual_aid.moderate',          'Moderate mutual aid posts (moderator+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key IN ('mutual_aid.create_offer','mutual_aid.create_skill_swap')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'mutual_aid.moderate'
ON CONFLICT DO NOTHING;

COMMIT;
