-- 000749_list_content_type.up.sql
-- List (listicle) content type — §14 amendment.
-- Seeds list.create (super_admin only; admins opt other groups in via the
-- RBAC admin UI) and agent.content_type.list (granted to every role that
-- already holds the agent.content_type family, mirroring 000561). Also seeds
-- the ItemList JSON-LD template per §14's admin-template design.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('list.create', 'Create list (listicle) content via the composer or POST /content'),
    ('agent.content_type.list', 'AI Agent — create list content')
ON CONFLICT (key) DO NOTHING;

-- list.create → super-admin roles only. NOT granted to member/admin-group-role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key = 'list.create'
ON CONFLICT DO NOTHING;

-- agent.content_type.list → every role already holding the agent.content_type
-- family (000561 granted it to member + super_admin). Derive the grant set
-- from an existing family member so future roles stay consistent.
INSERT INTO role_permissions (role_id, permission_id)
SELECT DISTINCT rp.role_id, p.id
FROM role_permissions rp
JOIN permissions src ON src.id = rp.permission_id AND src.key = 'agent.content_type.blog_post'
JOIN permissions p ON p.key = 'agent.content_type.list'
ON CONFLICT DO NOTHING;

-- ItemList JSON-LD template. itemListElement/numberOfItems are injected at
-- render time by the content handler (the {{field}} interpolator only emits
-- strings, so list items cannot be expressed in the template itself).
INSERT INTO content_type_jsonld_templates (content_type, template, is_active) VALUES (
    'list',
    '{"@context":"https://schema.org","@type":"ItemList","name":"{{title}}","description":"{{description}}","url":"{{canonical_url}}","image":"{{cover_image_url}}","author":{"@type":"Person","name":"{{author_name}}"},"datePublished":"{{published_at}}","dateModified":"{{updated_at}}"}'::jsonb,
    true
);

COMMIT;
