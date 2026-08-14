-- Down migration for 000051_newsletter.sql
-- Drops newsletter campaigns and subscribers tables.
DROP TABLE IF EXISTS newsletter_campaigns;
DROP TABLE IF EXISTS newsletter_subscribers;
