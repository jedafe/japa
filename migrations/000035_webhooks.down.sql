-- Down migration for 000035_webhooks.sql
-- Drops webhook deliveries and endpoints tables.
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhook_endpoints;
