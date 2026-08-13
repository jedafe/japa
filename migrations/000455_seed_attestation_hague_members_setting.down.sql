-- 000455_seed_attestation_hague_members_setting.down.sql
-- Revert BUG-002 fix: remove the hague_members system_setting.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'attestation_guide.hague_members';

COMMIT;
