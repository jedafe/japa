-- 000215_seed_docs_learn_footer_menus.up.sql
-- Seeds footer menus for the docs and learn apps so admins can manage their
-- footer links from the admin panel (Rule 16 — no hardcoded frontend links).
-- Ref: ADR-054 (Navigation Menus)

BEGIN;

INSERT INTO menus (tenant_id, slug, name, placement) VALUES
    ('00000000-0000-0000-0000-000000000001', 'docs_footer', 'Docs Footer', 'footer'),
    ('00000000-0000-0000-0000-000000000001', 'learn_footer', 'Learn Footer', 'footer')
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Docs Footer items
INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Docs Home', '/docs', NULL, 0, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Getting Started', '/docs/getting-started', NULL, 1, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'API Reference', '/docs/api', NULL, 2, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Guides', '/docs/guides', NULL, 3, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Main App', '/', NULL, 4, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Privacy', '/privacy', NULL, 5, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Terms', '/terms', NULL, 6, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Sign up', '/signup', NULL, 7, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Log in', '/login', NULL, 8, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- Learn Footer items
INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Explore', '/', NULL, 0, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'My Learning', '/my-learning', NULL, 1, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Certificates', '/certificates', NULL, 2, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Main App', '/', NULL, 3, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Docs', '/docs', NULL, 4, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Privacy', '/privacy', NULL, 5, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Terms', '/terms', NULL, 6, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Sign up', '/signup', NULL, 7, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Log in', '/login', NULL, 8, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
