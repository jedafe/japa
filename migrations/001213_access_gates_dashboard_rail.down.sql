-- 001213_access_gates_dashboard_rail.down.sql
-- Remove the access-gates dashboard rail item.

BEGIN;

DELETE FROM menu_items
WHERE label = 'dashboard.nav.access_gates'
  AND url = '/dashboard/access-gates';

COMMIT;
