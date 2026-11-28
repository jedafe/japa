-- 001240_subscription_plans_integrity.down.sql
-- Revert the subscription_plans CHECK constraints added in the up file.

BEGIN;

ALTER TABLE subscription_plans
    DROP CONSTRAINT subscription_plans_status_check,
    DROP CONSTRAINT subscription_plans_slug_check,
    DROP CONSTRAINT subscription_plans_amounts_check,
    DROP CONSTRAINT subscription_plans_currency_check;

COMMIT;
