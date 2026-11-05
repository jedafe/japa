-- 001051 — KYC submission records (SEC-W-015b pipeline spine).
--
-- users.kyc_status (001045) is the decision flag; this table records the
-- submission that justifies it — users submit evidence, admins decide
-- via POST /admin/users/{id}/kyc, rows here are the audit trail.

BEGIN;

CREATE TABLE IF NOT EXISTS user_kyc_submissions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document_type text NOT NULL,        -- passport | national_id | drivers_license | proof_of_address | other
    document_ref  text,                 -- storage path / provider ref — not the document itself
    note          text,
    status        text NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending','approved','rejected','superseded')),
    submitted_at  timestamptz NOT NULL DEFAULT now(),
    decided_at    timestamptz,
    decided_by    uuid REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_user_kyc_submissions_user ON user_kyc_submissions (user_id, submitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_kyc_submissions_status ON user_kyc_submissions (status) WHERE status = 'pending';

COMMIT;
