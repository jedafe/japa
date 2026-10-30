-- 000959_tips_checkout_url.up.sql
-- Persists the provider checkout URL on the tip so an idempotent replay of a
-- pending fiat tip can resume checkout instead of dead-ending on "pending".
BEGIN;
ALTER TABLE public.tips ADD COLUMN IF NOT EXISTS checkout_url text;
COMMIT;
