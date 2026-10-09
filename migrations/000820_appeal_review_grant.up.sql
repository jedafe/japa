-- 000820_appeal_review_grant.up.sql — §22: register and grant the
-- moderation.appeal.review node used by the admin appeal-review routes
-- (GET/PATCH /admin/appeals). Granted to the super_admin roles following
-- the same convention as moderation.alerts.manage (migration 000122);
-- other roles can receive it via the RBAC admin UI.

INSERT INTO permissions (key, description) VALUES
    ('moderation.appeal.review', 'Moderation Appeal Review')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key = 'moderation.appeal.review'
ON CONFLICT (role_id, permission_id) DO NOTHING;
