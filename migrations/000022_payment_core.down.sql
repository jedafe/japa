-- Down migration for 000022_payment_core.sql
-- Drops payment refunds, discount code uses, discount codes, subscriptions,
-- payment webhook events, and payments tables.
DROP TABLE IF EXISTS payment_refunds;
DROP TABLE IF EXISTS discount_code_uses;
DROP TABLE IF EXISTS discount_codes;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS payment_webhook_events;
DROP TABLE IF EXISTS payments;
