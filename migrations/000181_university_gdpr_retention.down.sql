-- Revert: restore original CASCADE FKs and NOT NULL constraints.
ALTER TABLE revenue_splits
    DROP CONSTRAINT IF EXISTS revenue_splits_instructor_id_fkey;

ALTER TABLE revenue_splits
    ADD CONSTRAINT revenue_splits_instructor_id_fkey
    FOREIGN KEY (instructor_id) REFERENCES instructor_profiles(id)
    ON DELETE CASCADE;

-- NOTE: cannot re-add NOT NULL if any NULL rows exist from the up migration.
-- This down migration assumes a clean rollback before any GDPR erasure runs.
ALTER TABLE revenue_splits
    ALTER COLUMN instructor_id SET NOT NULL;

ALTER TABLE instructor_payouts
    DROP CONSTRAINT IF EXISTS instructor_payouts_instructor_id_fkey;

ALTER TABLE instructor_payouts
    ADD CONSTRAINT instructor_payouts_instructor_id_fkey
    FOREIGN KEY (instructor_id) REFERENCES instructor_profiles(id)
    ON DELETE CASCADE;

ALTER TABLE instructor_payouts
    ALTER COLUMN instructor_id SET NOT NULL;
