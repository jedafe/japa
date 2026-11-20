-- 001213_access_gates_dashboard_rail.down.sql
-- Remove the access-gates dashboard rail item.

BEGIN;

DELETE FROM menu_items
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND label = 'dashboard.nav.access_gates'
  AND url = '/dashboard/access-gates';

COMMIT;
