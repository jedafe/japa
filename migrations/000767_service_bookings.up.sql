-- 000767_service_bookings.up.sql
-- §111 / ADR-123 — unified leads + bookings pipeline for the whole service
-- directory. intent='inquiry' rows are leads; intent='booking' rows carry a
-- platform-fee payment (purpose_type='service_booking_fee').
--
-- Also extends the payments.purpose_type CHECK constraint (originally
-- migration 000358) with 'service_booking_fee'.
--
-- Ref: .jeos/sections/111-service-packages-bookings.md

BEGIN;

CREATE TABLE IF NOT EXISTS service_bookings (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    listing_meta_id   uuid NOT NULL REFERENCES service_listing_meta(id),
    departure_id      uuid REFERENCES service_package_departures(id) ON DELETE SET NULL,
    -- NULLable + SET NULL: GDPR erasure (§19) anonymizes then hard-deletes
    -- the user row; the booking row survives for the org's audit trail.
    booker_user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    org_id            uuid NOT NULL REFERENCES organizations(id),
    intent            text NOT NULL CHECK (intent IN ('inquiry','booking')),
    party_size        int NOT NULL DEFAULT 1,
    preferred_dates   jsonb,
    note              text,
    status            text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','confirmed','fulfilled','cancelled','declined','disputed','refunded')),
    purchase_mode     text NOT NULL,
    payment_id        uuid REFERENCES payments(id),
    -- Financial snapshots taken at booking time; bigint minor units
    -- matching payments.amount. Never recomputed — the listing price may
    -- change after the booking is placed.
    total_price       bigint,
    fee_amount        bigint,
    fee_currency      text,
    agent_balance_due bigint,
    org_response_note text,
    expires_at        timestamptz,
    confirmed_at      timestamptz,
    fulfilled_at      timestamptz,
    cancelled_at      timestamptz,
    cancel_reason     text,
    -- Set when the booker's GDPR erasure anonymizes the row (§111 §19):
    -- booker_user_id is then a tombstone reference and note/preferred_dates
    -- are wiped. The row survives for the org's audit trail.
    anonymized_at     timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_svc_bookings_org
    ON service_bookings (org_id, status, intent);
CREATE INDEX IF NOT EXISTS idx_svc_bookings_user
    ON service_bookings (booker_user_id, status);
CREATE INDEX IF NOT EXISTS idx_svc_bookings_listing
    ON service_bookings (listing_meta_id, status);
-- service:booking_expire_pending sweep target.
CREATE INDEX IF NOT EXISTS idx_svc_bookings_expiry
    ON service_bookings (expires_at) WHERE status = 'pending';

-- Extend the payment purpose enum (000358 CHECK) for platform-fee payments.
ALTER TABLE payments
    DROP CONSTRAINT payments_purpose_type_check,
    ADD CONSTRAINT payments_purpose_type_check
        CHECK (purpose_type = ANY (ARRAY[
            'pro_membership','org_boost','donation','course_access',
            'credit_purchase','ad_slot','form_submission','service_booking_fee'
        ]));

COMMIT;
