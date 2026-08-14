-- Down migration for 000044_v2_groundwork.sql
-- Drops V2 groundwork tables: external data sources, bookings, ads,
-- mentorship, and newsletter tables.
DROP TABLE IF EXISTS external_data_sources;
DROP TABLE IF EXISTS booking_slots;
DROP TABLE IF EXISTS bookings;
DROP TABLE IF EXISTS ads_campaigns;
DROP TABLE IF EXISTS mentor_reviews;
DROP TABLE IF EXISTS mentor_sessions;
DROP TABLE IF EXISTS mentorship_profiles;
DROP TABLE IF EXISTS newsletter_list_subscribers;
DROP TABLE IF EXISTS newsletter_subscribers;
DROP TABLE IF EXISTS newsletter_lists;
