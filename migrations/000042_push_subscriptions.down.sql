-- Down migration for 000042_push_subscriptions.sql
-- Drops the push_subscriptions table for web push notification subscriptions.
DROP TABLE IF EXISTS push_subscriptions;
