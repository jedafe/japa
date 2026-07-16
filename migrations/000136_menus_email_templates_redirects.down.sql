-- 000136_menus_email_templates_redirects.down.sql
-- Rolls back ADR-054 entity creation.

BEGIN;

DROP TABLE IF EXISTS redirects CASCADE;
DROP TABLE IF EXISTS email_templates CASCADE;
DROP TABLE IF EXISTS menu_items CASCADE;
DROP TABLE IF EXISTS menus CASCADE;

COMMIT;
