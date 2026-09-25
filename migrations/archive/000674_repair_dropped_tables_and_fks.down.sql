-- 000674_repair_dropped_tables_and_fks.down.sql
-- Drops the objects created by 000674. Restores the broken state (missing
-- tables / constraints) — run only when intentionally reverting.

BEGIN;

ALTER TABLE reactions DROP CONSTRAINT IF EXISTS reactions_reaction_key_fkey;
DROP INDEX IF EXISTS referrals_referee_id_key;

DROP TABLE IF EXISTS content_engagement;
DROP TABLE IF EXISTS external_data_sources;
DROP TABLE IF EXISTS booking_slots;
DROP TABLE IF EXISTS bookings;
DROP TABLE IF EXISTS ads_campaigns;
DROP TABLE IF EXISTS mentor_reviews;
DROP TABLE IF EXISTS mentor_sessions;
DROP TABLE IF EXISTS mentorship_profiles;
DROP TABLE IF EXISTS newsletter_list_subscribers;
DROP TABLE IF EXISTS newsletter_lists;
-- newsletter_subscribers predates this repair (000044/000051) — left in place.
DROP TABLE IF EXISTS push_subscriptions;

COMMIT;
