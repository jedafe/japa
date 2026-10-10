-- 000828_mentorship_booking_invariants.down.sql
-- Reverses all three tightenings: restores the meta-keyed dedupe index,
-- the wider anonymized_at escape on the provider CHECK, and drops the
-- one-meta-per-content unique.

BEGIN;

-- c) Restore the 000825 meta-keyed dedupe index.
DROP INDEX IF EXISTS idx_svc_bookings_mentorship_open_uniq;
CREATE UNIQUE INDEX idx_svc_bookings_mentorship_open_uniq
    ON service_bookings (booker_user_id, listing_meta_id)
    WHERE org_id IS NULL AND status IN ('pending', 'confirmed');

-- b) Restore the wider provider-check escape (any anonymized row may
--    violate the XOR).
ALTER TABLE service_bookings
    DROP CONSTRAINT service_bookings_provider_check,
    ADD CONSTRAINT service_bookings_provider_check
        CHECK ((org_id IS NOT NULL) <> (provider_user_id IS NOT NULL)
               OR anonymized_at IS NOT NULL);

-- a) Drop the one-meta-per-content unique.
DROP INDEX IF EXISTS idx_service_listing_meta_content_id_uniq;

COMMIT;
