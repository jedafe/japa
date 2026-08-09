-- Down migration for 000358: revert CHECK constraints to their pre-fix state.

ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_purpose_type_check;
ALTER TABLE payments
    ADD CONSTRAINT payments_purpose_type_check
    CHECK (purpose_type IN ('pro_membership','org_boost','donation','course_access','credit_purchase'));

ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_status_check;
ALTER TABLE payments
    ADD CONSTRAINT payments_status_check
    CHECK (status IN ('pending','processing','succeeded','failed','refunded','cancelled'));
