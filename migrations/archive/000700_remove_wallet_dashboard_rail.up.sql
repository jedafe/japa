-- 000700_remove_wallet_dashboard_rail.up.sql
-- Renumbered from 000600 — it collided with 000600_learning_slugs_url_safe.
-- De-tenanted form: menu_items.tenant_id was dropped by 000668, so the tenant
-- filter is gone. Tail-position body; databases that applied the 000600 file
-- skip this via the 000000 registry marker.

BEGIN;

DELETE FROM menu_items
WHERE url = '/dashboard/wallet';

COMMIT;
