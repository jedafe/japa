-- 000222_sidebar_blog_content_seeds.down.sql
-- Remove seeded blog/content right sidebars and their widgets.
-- Ref: Section 59 (Sidebar Management System), ADR-066.

BEGIN;

DELETE FROM sidebars
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND slug IN ('main-blog-right', 'main-content-right');

COMMIT;
