-- Down migration for 000056_referral.sql
-- Drops referral tracking and referral program tables.
DROP TABLE IF EXISTS referrals;
DROP TABLE IF EXISTS referral_programs;
