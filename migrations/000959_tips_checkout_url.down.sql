BEGIN;
ALTER TABLE public.tips DROP COLUMN checkout_url;
COMMIT;
