-- Down migration for 000023_wallet_credits.sql
-- Drops wallet settings, activity events, activity point rules, and wallet ledger tables.
DROP TABLE IF EXISTS wallet_settings;
DROP TABLE IF EXISTS activity_events;
DROP TABLE IF EXISTS activity_point_rules;
DROP TABLE IF EXISTS wallet_ledger;
