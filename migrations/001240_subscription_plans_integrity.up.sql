-- 001240_subscription_plans_integrity.up.sql
-- §17 Payment Core: tighten subscription_plans at the DB layer. The admin
-- CRUD endpoints now validate input (slug pattern, status, tier, amounts,
-- currency), but the table itself had no CHECKs beyond tier — a direct
-- write could still insert negative amounts, a non-standard status, or a
-- slug that can never map to a subscription plan code.
--
-- Status domain is active|inactive: ListActiveSubscriptionPlans serves
-- status='active' only, and the admin UI offers exactly those two values.
-- Slug is constrained to the same pattern the API validates because it is
-- interpolated into plan codes ({slug}_{monthly|yearly}) and provider
-- settings keys.
--
-- Ref: .jeos/sections/17-payment-core.md
--      .jeos/audit/pricing-audit.md (BUG-PR-007 follow-up)

BEGIN;

ALTER TABLE subscription_plans
    ADD CONSTRAINT subscription_plans_status_check
        CHECK (status = ANY (ARRAY['active'::text, 'inactive'::text])),
    ADD CONSTRAINT subscription_plans_slug_check
        CHECK (slug ~ '^[a-z0-9][a-z0-9_-]{0,63}$'),
    ADD CONSTRAINT subscription_plans_amounts_check
        CHECK (monthly_amount >= 0 AND yearly_amount >= 0),
    ADD CONSTRAINT subscription_plans_currency_check
        CHECK (currency ~ '^[A-Za-z]{3}$');

COMMIT;
