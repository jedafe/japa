-- 000287_reactivate_deleted_geo_spaces.up.sql
-- Reactivate any soft-deleted country/state geo spaces that were skipped by
-- migration 000286 (ON CONFLICT DO NOTHING left them in the 'deleted' state
-- because the row already existed). The geo list endpoints filter with
-- deleted_at IS NULL, so these spaces were invisible in the admin /geo page.
--
-- This restores them to active status with cleared deleted_at.

UPDATE spaces
SET status = 'active', deleted_at = NULL
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND level IN ('country', 'state')
  AND deleted_at IS NOT NULL;
