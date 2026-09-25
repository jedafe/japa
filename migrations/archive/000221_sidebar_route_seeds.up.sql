-- 000221_sidebar_route_seeds.up.sql
-- Seed all route-specific right sidebars across Main, Docs, and Learn apps.
-- Ref: Section 59 (Sidebar Management System), ADR-066.
--
-- The Main app already has main-default-right (migration 000220).
-- This migration seeds:
--   Main:   main-user-right (/u/*), main-space-right (/spaces/*),
--           main-org-right (/org/*), main-tag-right (/tags/*)
--   Docs:   docs-default-right (*)
--   Learn:  learn-default-right (*), learn-lesson-right (/tracks/*)
--
-- Route-specific sidebars (user, space, org, tag, lesson) get content-slot
-- widgets only — the route-specific snippet (setSidebar) provides the primary
-- content, and the sidebar system widgets render below it.
-- Default sidebars (docs, learn) get content-slot widgets that supplement
-- the existing hardcoded topbar widgets.

BEGIN;

-- ── Helper: insert a sidebar if it doesn't exist for the default tenant ──
-- We use deterministic UUIDs for idempotency.

-- ════════════════════════════════════════════════════════════════════════
-- Main app — route-specific right sidebars
-- ════════════════════════════════════════════════════════════════════════

-- User profile pages: /u/[username]
INSERT INTO sidebars (id, tenant_id, app, position, slug, name, page_pattern, visibility, status)
SELECT 'a1b2c3d4-0002-4000-8000-000000000001'::uuid, t.id, 'main', 'right', 'main-user-right', 'User Profile Right', '/u/*', 'all', 'active'
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM sidebars WHERE tenant_id = t.id AND app='main' AND position='right' AND slug='main-user-right')
  AND t.id = '00000000-0000-0000-0000-000000000001'::uuid;

-- Space detail pages: /spaces/[slug]
INSERT INTO sidebars (id, tenant_id, app, position, slug, name, page_pattern, visibility, status)
SELECT 'a1b2c3d4-0002-4000-8000-000000000002'::uuid, t.id, 'main', 'right', 'main-space-right', 'Space Detail Right', '/spaces/*', 'all', 'active'
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM sidebars WHERE tenant_id = t.id AND app='main' AND position='right' AND slug='main-space-right')
  AND t.id = '00000000-0000-0000-0000-000000000001'::uuid;

-- Organization profile pages: /org/[slug]
INSERT INTO sidebars (id, tenant_id, app, position, slug, name, page_pattern, visibility, status)
SELECT 'a1b2c3d4-0002-4000-8000-000000000003'::uuid, t.id, 'main', 'right', 'main-org-right', 'Organization Profile Right', '/org/*', 'all', 'active'
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM sidebars WHERE tenant_id = t.id AND app='main' AND position='right' AND slug='main-org-right')
  AND t.id = '00000000-0000-0000-0000-000000000001'::uuid;

-- Tag detail pages: /tags/[slug]
INSERT INTO sidebars (id, tenant_id, app, position, slug, name, page_pattern, visibility, status)
SELECT 'a1b2c3d4-0002-4000-8000-000000000004'::uuid, t.id, 'main', 'right', 'main-tag-right', 'Tag Detail Right', '/tags/*', 'all', 'active'
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM sidebars WHERE tenant_id = t.id AND app='main' AND position='right' AND slug='main-tag-right')
  AND t.id = '00000000-0000-0000-0000-000000000001'::uuid;

-- ════════════════════════════════════════════════════════════════════════
-- Docs app — default right sidebar
-- ════════════════════════════════════════════════════════════════════════

INSERT INTO sidebars (id, tenant_id, app, position, slug, name, page_pattern, visibility, status)
SELECT 'a1b2c3d4-0003-4000-8000-000000000001'::uuid, t.id, 'docs', 'right', 'docs-default-right', 'Docs Default Right', '*', 'all', 'active'
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM sidebars WHERE tenant_id = t.id AND app='docs' AND position='right' AND slug='docs-default-right')
  AND t.id = '00000000-0000-0000-0000-000000000001'::uuid;

-- ════════════════════════════════════════════════════════════════════════
-- Learn app — default + lesson right sidebars
-- ════════════════════════════════════════════════════════════════════════

INSERT INTO sidebars (id, tenant_id, app, position, slug, name, page_pattern, visibility, status)
SELECT 'a1b2c3d4-0004-4000-8000-000000000001'::uuid, t.id, 'learn', 'right', 'learn-default-right', 'Learn Default Right', '*', 'all', 'active'
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM sidebars WHERE tenant_id = t.id AND app='learn' AND position='right' AND slug='learn-default-right')
  AND t.id = '00000000-0000-0000-0000-000000000001'::uuid;

INSERT INTO sidebars (id, tenant_id, app, position, slug, name, page_pattern, visibility, status)
SELECT 'a1b2c3d4-0004-4000-8000-000000000002'::uuid, t.id, 'learn', 'right', 'learn-lesson-right', 'Learn Lesson Right', '/tracks/*', 'all', 'active'
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM sidebars WHERE tenant_id = t.id AND app='learn' AND position='right' AND slug='learn-lesson-right')
  AND t.id = '00000000-0000-0000-0000-000000000001'::uuid;

-- ════════════════════════════════════════════════════════════════════════
-- Seed widgets for each sidebar
-- ════════════════════════════════════════════════════════════════════════

-- Main app route-specific sidebars: ad_slot (top) + trending_tags + ad_slot (bottom)
-- These render BELOW the route-specific snippet (UserProfileSidebar, SpaceSidebar, etc.)
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
  AND s.slug IN ('main-user-right', 'main-space-right', 'main-org-right', 'main-tag-right')
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets sw
      WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type AND sw.sort_order = wt.sort_order
  );

-- Docs default sidebar: ad_slot (top) + rich_text (admin-managed content)
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, 'all', 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('ad_slot',   'Ad Slot',     '{"slot":"sidebar_top","format":"square"}'::jsonb,  0),
    ('rich_text', 'Sidebar Text','{"content":"<p>Admin-managed sidebar content for Docs.</p>"}'::jsonb, 1)
) AS wt(widget_type, label, props, sort_order)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'docs' AND s.position = 'right' AND s.slug = 'docs-default-right'
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets sw
      WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type AND sw.sort_order = wt.sort_order
  );

-- Learn default sidebar: ad_slot (top) + rich_text
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, 'all', 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('ad_slot',   'Ad Slot',     '{"slot":"sidebar_top","format":"square"}'::jsonb,  0),
    ('rich_text', 'Sidebar Text','{"content":"<p>Admin-managed sidebar content for Learn.</p>"}'::jsonb, 1)
) AS wt(widget_type, label, props, sort_order)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'learn' AND s.position = 'right' AND s.slug = 'learn-default-right'
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets sw
      WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type AND sw.sort_order = wt.sort_order
  );

-- Learn lesson sidebar: ad_slot only (the lesson player snippet provides the main content)
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, 'all', 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('ad_slot', 'Ad Slot', '{"slot":"sidebar_top","format":"square"}'::jsonb, 0)
) AS wt(widget_type, label, props, sort_order)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'learn' AND s.position = 'right' AND s.slug = 'learn-lesson-right'
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets sw
      WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type AND sw.sort_order = wt.sort_order
  );

COMMIT;
