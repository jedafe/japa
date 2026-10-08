-- 000816_ad_campaign_last_accrual.down.sql

ALTER TABLE public.ad_campaigns DROP COLUMN last_accrual_date;
