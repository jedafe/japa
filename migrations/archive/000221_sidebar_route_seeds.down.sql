-- 000221_sidebar_route_seeds.down.sql
-- Remove all seeded route-specific sidebars and their widgets.
-- Ref: Section 59 (Sidebar Management System), ADR-066.

BEGIN;

-- Widgets are cascaded by the FK (sidebar_widgets.sidebar_id → sidebars.id ON DELETE CASCADE).
DELETE FROM sidebars
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND slug IN (
    'main-user-right', 'main-space-right', 'main-org-right', 'main-tag-right',
    'docs-default-right',
    'learn-default-right', 'learn-lesson-right'
  );

COMMIT;
