-- service_bookings dedupe: scope the open-booking guard per listing, not
-- per (booker, provider) pair.
--
-- The pair-keyed index (000828) predates author-provider service
-- inquiries (listings without an org_id deliver to the listing author's
-- provider inbox). Keying on the pair meant one open inquiry to an
-- author blocked inquiries on that author's OTHER listings.
--
-- Mentorship semantics are unchanged: a mentor's shadow listing is 1:1
-- per mentor, so (booker, provider, listing_meta_id) reduces to the same
-- (booker, provider) pair for mentorship rows.

DROP INDEX idx_svc_bookings_mentorship_open_uniq;
CREATE UNIQUE INDEX idx_svc_bookings_mentorship_open_uniq
    ON service_bookings (booker_user_id, provider_user_id, listing_meta_id)
    WHERE org_id IS NULL AND status IN ('pending', 'confirmed');
