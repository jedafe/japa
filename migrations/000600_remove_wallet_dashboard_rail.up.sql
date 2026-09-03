-- 000600_remove_wallet_dashboard_rail.up.sql
-- Remove the wallet item from the authenticated dashboard rail.
-- The wallet surface now lives in /settings/wallet.

BEGIN;

DELETE FROM menu_items
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND url = '/dashboard/wallet';

COMMIT;
