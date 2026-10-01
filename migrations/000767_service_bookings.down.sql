-- 000767_service_bookings.down.sql
BEGIN;

ALTER TABLE payments
    DROP CONSTRAINT payments_purpose_type_check,
    ADD CONSTRAINT payments_purpose_type_check
        CHECK (purpose_type = ANY (ARRAY[
            'pro_membership','org_boost','donation','course_access',
            'credit_purchase','ad_slot','form_submission'
        ]));

DROP TABLE IF EXISTS service_bookings;

COMMIT;
