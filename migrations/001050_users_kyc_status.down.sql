BEGIN;

DELETE FROM system_settings
WHERE key IN ('wallet.withdrawal_verification_level', 'wallet.admin_approval_requires_step_up');

ALTER TABLE users DROP COLUMN IF EXISTS kyc_status;

COMMIT;
