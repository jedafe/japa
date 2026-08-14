-- Down migration for 000024_advertising.sql
-- Drops organization boosts, ad analytics, ad events, ad campaigns, and ad slot types tables.
DROP TABLE IF EXISTS organization_boosts;
DROP TABLE IF EXISTS ad_analytics_daily;
DROP TABLE IF EXISTS ad_events;
DROP TABLE IF EXISTS ad_campaigns;
DROP TABLE IF EXISTS ad_slot_types;
