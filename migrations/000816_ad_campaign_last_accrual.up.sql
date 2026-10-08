-- 000816_ad_campaign_last_accrual.up.sql — §25 amendment.
-- Per-campaign idempotency marker for ad:accrue_spend_daily. Replaces the
-- Valkey jc:ad_accrue:{date} day-lock, which skipped the whole run on retry
-- (including campaigns that failed mid-loop). With this column, a retried
-- run no-ops only on campaigns already accrued today.

ALTER TABLE public.ad_campaigns
    ADD COLUMN last_accrual_date date;
