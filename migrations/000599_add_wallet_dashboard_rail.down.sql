-- 000599_add_wallet_dashboard_rail.down.sql
-- Remove the wallet dashboard rail item.

BEGIN;

DELETE FROM menu_items
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND label = 'dashboard.nav.wallet'
  AND url = '/dashboard/wallet';

COMMIT;
