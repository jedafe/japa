-- 000605_add_feature_withdraw_flag.down.sql
-- Remove the feature.withdraw kill switch.

BEGIN;

DELETE FROM feature_flags
WHERE key = 'feature.withdraw' AND tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
