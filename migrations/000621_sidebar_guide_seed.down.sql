-- 000621_sidebar_guide_seed.down.sql
-- Remove the seeded /guide/* right sidebar and its widgets.
-- Ref: Section 59 (Sidebar Management System), ADR-066.

BEGIN;

DELETE FROM sidebars
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND slug = 'main-guide-right';

COMMIT;
