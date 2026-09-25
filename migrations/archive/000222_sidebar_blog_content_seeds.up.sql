-- 000222_sidebar_blog_content_seeds.up.sql
-- Seed right sidebars for /blog/[slug] and /content/[slug] reading pages.
-- Ref: Section 59 (Sidebar Management System), ADR-066, ADR-058 Exception #1.
--
-- These pages use a custom dev.to-style 3-column layout where the
-- CreatorCard replaces the standard topbar. Admin-managed widgets
-- render below the CreatorCard in the same <aside> column.

BEGIN;

-- Blog post pages: /blog/[slug]
INSERT INTO sidebars (id, tenant_id, app, position, slug, name, page_pattern, visibility, status)
SELECT 'a1b2c3d4-0002-4000-8000-000000000005'::uuid, t.id, 'main', 'right', 'main-blog-right', 'Blog Post Right', '/blog/*', 'all', 'active'
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM sidebars WHERE tenant_id = t.id AND app='main' AND position='right' AND slug='main-blog-right')
  AND t.id = '00000000-0000-0000-0000-000000000001'::uuid;

-- Content article pages: /content/[slug]
INSERT INTO sidebars (id, tenant_id, app, position, slug, name, page_pattern, visibility, status)
SELECT 'a1b2c3d4-0002-4000-8000-000000000006'::uuid, t.id, 'main', 'right', 'main-content-right', 'Content Article Right', '/content/*', 'all', 'active'
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM sidebars WHERE tenant_id = t.id AND app='main' AND position='right' AND slug='main-content-right')
  AND t.id = '00000000-0000-0000-0000-000000000001'::uuid;

-- Seed widgets: ad_slot (top) + trending_tags + ad_slot (bottom)
-- These render below the CreatorCard in the reading layout's <aside>.
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, 'all', 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('ad_slot',       'Ad Top',         '{"slot":"sidebar_top","format":"square"}'::jsonb,  0),
    ('trending_tags', 'Trending Tags',  '{"limit":8,"title":"Trending Tags"}'::jsonb,      1),
    ('ad_slot',       'Ad Bottom',      '{"slot":"sidebar_bottom","format":"banner"}'::jsonb, 2)
) AS wt(widget_type, label, props, sort_order)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'main' AND s.position = 'right'
  AND s.slug IN ('main-blog-right', 'main-content-right')
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets sw
      WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type AND sw.sort_order = wt.sort_order
  );

COMMIT;
