-- 000825_mentorship_booking_dedupe.down.sql
-- Drop the open-mentorship-request dedupe guard; the servicedir
-- SELECT-then-INSERT pre-check remains (racy again without the index).

DROP INDEX IF EXISTS idx_svc_bookings_mentorship_open_uniq;
