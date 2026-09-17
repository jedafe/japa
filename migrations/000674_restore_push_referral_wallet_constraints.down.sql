-- 000674_restore_push_referral_wallet_constraints.down.sql
-- Reverses 000674.

BEGIN;

DROP TABLE IF EXISTS push_subscriptions;

DROP INDEX IF EXISTS referrals_referee_id_key;
DROP INDEX IF EXISTS referral_programs_singleton;
DROP INDEX IF EXISTS wallet_settings_singleton;

COMMIT;
