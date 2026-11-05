-- 001050 — platform-wide KYC flag on users (SEC-W-015b).
--
-- instructor_profiles.kyc_status is academy-only; withdrawals need a
-- user-level verification signal. kyc_status follows the same state
-- vocabulary: unverified → pending → verified | failed.
-- Set via POST /admin/users/{id}/kyc (admin.users.manage).

BEGIN;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS kyc_status text NOT NULL DEFAULT 'unverified'
    CONSTRAINT users_kyc_status_check
    CHECK (kyc_status IN ('unverified', 'pending', 'verified', 'failed'));

-- Verification level for withdrawals: 'none' (gate off), 'email'
-- (email_verified_at required — default), 'kyc' (kyc_status='verified'
-- required — strongest, flip when the KYC pipeline is staffed).
-- wallet.admin_approval_requires_step_up: admin withdrawal
-- approve/reject requires the admin's own step_up token.
INSERT INTO system_settings (key, value)
VALUES
    ('wallet.withdrawal_verification_level', '"email"'::jsonb),
    ('wallet.admin_approval_requires_step_up', 'true'::jsonb)
ON CONFLICT (key) DO NOTHING;

COMMIT;
