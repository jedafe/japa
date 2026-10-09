-- 000825_mentorship_booking_dedupe.up.sql
-- One open mentorship request per (booker, mentor shadow listing).
-- createMentorshipBooking pre-checks via GetOpenMentorshipBooking, but a
-- SELECT-then-INSERT race lets two concurrent requests both pass the probe —
-- this partial unique index is the DB-level guard; the servicedir create
-- path maps its 23505 to ErrDuplicateRequest (409 duplicate_mentorship_request).
--
-- Predicate parity: org_id IS NULL identifies exactly the user-owned
-- bookings under the 000824 provider XOR CHECK — org-owned bookings keep
-- multi-inquiry freedom; pending|confirmed matches the open-status set
-- GetOpenMentorshipBooking dedupes on.
--
-- Ref: docs/superpowers/plans/2026-10-09-services-mentorship-unification.md

CREATE UNIQUE INDEX idx_svc_bookings_mentorship_open_uniq
    ON service_bookings (booker_user_id, listing_meta_id)
    WHERE org_id IS NULL AND status IN ('pending', 'confirmed');
