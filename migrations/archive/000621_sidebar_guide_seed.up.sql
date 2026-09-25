-- 000621_sidebar_guide_seed.up.sql
-- Seed a right sidebar for /guide/[slug] reading pages (canonical route
-- family for content_type 'guide', §14 / ADR-058). Guide singles share the
-- same dev.to-style reading layout as /content/[slug] — the widget set is
-- copied from main-content-right at apply time so admin customizations to
-- the content-single chrome carry over.
-- Ref: Section 59 (Sidebar Management System), ADR-066, ADR-058 Exception #1.

BEGIN;

INSERT INTO sidebars (id, tenant_id, app, position, slug, name, page_pattern, visibility, status)
SELECT 'a1b2c3d4-0002-4000-8000-00000000000a'::uuid, t.id, 'main', 'right', 'main-guide-right', 'Guide Article Right', '/guide/*', 'all', 'active'
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM sidebars WHERE tenant_id = t.id AND app='main' AND position='right' AND slug='main-guide-right')
  AND t.id = '00000000-0000-0000-0000-000000000001'::uuid;

INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, required_group_ids, permission_node, status)
SELECT sw.tenant_id,
       g.id,
       sw.widget_type,
       sw.label,
       sw.props,
       sw.sort_order,
       sw.visibility,
       sw.required_group_ids,
       sw.permission_node,
       sw.status
FROM sidebar_widgets sw
JOIN sidebars c ON c.id = sw.sidebar_id AND c.slug = 'main-content-right' AND c.app = 'main' AND c.position = 'right'
JOIN sidebars g ON g.slug = 'main-guide-right' AND g.app = 'main' AND g.position = 'right' AND g.tenant_id = c.tenant_id
WHERE c.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets dup
      WHERE dup.sidebar_id = g.id AND dup.widget_type = sw.widget_type AND dup.sort_order = sw.sort_order
  );

COMMIT;
