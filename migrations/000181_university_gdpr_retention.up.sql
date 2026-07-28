-- GDPR/NDPR erasure: revenue_splits and instructor_payouts are financial
-- audit records that must survive user deletion (regulatory retention).
-- The original FKs (000166) used ON DELETE CASCADE which destroyed the
-- audit trail when a user was hard-deleted. Switch to ON DELETE SET NULL
-- so the instructor reference is anonymized (severed) but the money
-- records survive.
--
-- Ref: ADR-063 (revenue_splits = immutable audit record)
--      .jeos/audit/backend-audit-2026-07-28.md C2

-- revenue_splits.instructor_id: drop CASCADE, add SET NULL, allow NULL.
ALTER TABLE revenue_splits
    ALTER COLUMN instructor_id DROP NOT NULL;

ALTER TABLE revenue_splits
    DROP CONSTRAINT IF EXISTS revenue_splits_instructor_id_fkey;

ALTER TABLE revenue_splits
    ADD CONSTRAINT revenue_splits_instructor_id_fkey
    FOREIGN KEY (instructor_id) REFERENCES instructor_profiles(id)
    ON DELETE SET NULL;

-- instructor_payouts.instructor_id: drop CASCADE, add SET NULL, allow NULL.
ALTER TABLE instructor_payouts
    ALTER COLUMN instructor_id DROP NOT NULL;

ALTER TABLE instructor_payouts
    DROP CONSTRAINT IF EXISTS instructor_payouts_instructor_id_fkey;

ALTER TABLE instructor_payouts
    ADD CONSTRAINT instructor_payouts_instructor_id_fkey
    FOREIGN KEY (instructor_id) REFERENCES instructor_profiles(id)
    ON DELETE SET NULL;

COMMENT ON COLUMN revenue_splits.instructor_id IS
    'Owning instructor; NULL after GDPR erasure (anonymized, regulatory retention).';
COMMENT ON COLUMN instructor_payouts.instructor_id IS
    'Owning instructor; NULL after GDPR erasure (anonymized, regulatory retention).';
