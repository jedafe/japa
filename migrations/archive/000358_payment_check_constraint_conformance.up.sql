-- Section 17: Payment Core CHECK constraint conformance fix.
-- Fixes BUG-P001 (purpose_type CHECK excludes 'ad_slot' and 'form_submission')
-- and BUG-P002 (status CHECK excludes 'disputed').
--
-- The codebase uses purpose_type='ad_slot' for ad slot purchases (§61) and
-- purpose_type='form_submission' for paid form submissions (§06). The
-- PaymentStatusDisputed constant is defined and tested but the status CHECK
-- did not include 'disputed'. Both omissions cause CHECK constraint
-- violations at insert/update time.
-- Ref: JEOS §17, ADR-012, ADR-079 (Self-Serve Ads), §06 (Form Builder).

-- Drop and recreate the payments.purpose_type CHECK with the full value set.
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_purpose_type_check;
ALTER TABLE payments
    ADD CONSTRAINT payments_purpose_type_check
    CHECK (purpose_type IN (
        'pro_membership','org_boost','donation','course_access',
        'credit_purchase','ad_slot','form_submission'
    ));

-- Drop and recreate the payments.status CHECK to include 'disputed'.
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_status_check;
ALTER TABLE payments
    ADD CONSTRAINT payments_status_check
    CHECK (status IN (
        'pending','processing','succeeded','failed','refunded','cancelled','disputed'
    ));
