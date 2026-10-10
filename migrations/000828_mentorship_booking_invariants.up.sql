-- 000828_mentorship_booking_invariants.up.sql
-- Three schema tightenings for the 000824 mentorship → service-layer
-- unification:
--   a) service_listing_meta gains UNIQUE(content_id) — the schema implies
--      one meta row per listing content and EnsureMentorshipShadowMeta's
--      NOT EXISTS guard is a SELECT-then-INSERT, so two concurrent ensure
--      paths could both pass the probe and mint a duplicate. Verified no
--      duplicate content_id rows existed at apply time.
--   b) service_bookings_provider_check escape is narrowed: the
--      anonymized_at clause exists only for GDPR-scrubbed rows (the
--      anonymize path always nulls provider_user_id; org-owned rows keep
--      org_id so the plain XOR still covers them). The previous form let
--      ANY row with anonymized_at set violate the XOR — e.g. a row with
--      both owner pointers populated.
--   c) the open-mentorship dedupe re-keys from listing_meta_id to
--      provider_user_id: opting out soft-deletes the shadow and opting
--      back in mints a NEW meta id, so meta-keyed dedupe lets a mentee
--      stack duplicate open requests across opt-out/opt-in cycles.
--      provider_user_id is the stable mentor identity — parity with the
--      legacy mentorship_connections UNIQUE(mentor_id, mentee_id, status)
--      invariant. NULLs stay distinct under the unique index, so
--      anonymized (NULL, NULL) shells never collide.
--
-- Ref: docs/superpowers/plans/2026-10-09-services-mentorship-unification.md

BEGIN;

-- a) One meta row per listing content.
CREATE UNIQUE INDEX idx_service_listing_meta_content_id_uniq
    ON service_listing_meta (content_id);

-- b) Narrow the provider-check escape to actually-scrubbed rows.
ALTER TABLE service_bookings
    DROP CONSTRAINT service_bookings_provider_check,
    ADD CONSTRAINT service_bookings_provider_check
        CHECK ((org_id IS NOT NULL) <> (provider_user_id IS NOT NULL)
               OR (anonymized_at IS NOT NULL AND provider_user_id IS NULL));

-- c) Dedupe per (booker, mentor) pair, not per listing meta.
DROP INDEX idx_svc_bookings_mentorship_open_uniq;
CREATE UNIQUE INDEX idx_svc_bookings_mentorship_open_uniq
    ON service_bookings (booker_user_id, provider_user_id)
    WHERE org_id IS NULL AND status IN ('pending', 'confirmed');

COMMIT;
