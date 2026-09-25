-- 000223_sidebar_default_widgets.down.sql
-- Remove the default topbar widgets added by 000223 and restore original sort_order.
-- Ref: Section 59 (Sidebar Management System), ADR-066.

BEGIN;

-- Remove the 4 topbar widgets from all affected sidebars.
DELETE FROM sidebar_widgets sw
USING sidebars s
WHERE sw.sidebar_id = s.id
  AND s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.position = 'right'
  AND sw.widget_type IN ('search', 'theme_toggle', 'language_toggle', 'user_dropdown')
  AND s.slug IN (
    'main-user-right', 'main-space-right', 'main-org-right', 'main-tag-right',
    'main-blog-right', 'main-content-right',
    'docs-default-right', 'learn-default-right', 'learn-lesson-right'
  );

-- Shift remaining content widgets back down by 4 to restore original sort_order.
UPDATE sidebar_widgets sw
SET sort_order = sw.sort_order - 4
FROM sidebars s
WHERE sw.sidebar_id = s.id
  AND s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.position = 'right'
  AND s.slug IN (
    'main-user-right', 'main-space-right', 'main-org-right', 'main-tag-right',
    'main-blog-right', 'main-content-right',
    'docs-default-right', 'learn-default-right', 'learn-lesson-right'
  );

COMMIT;
