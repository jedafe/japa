-- 001182_escrow_booking_unique.down.sql

BEGIN;

DROP INDEX IF EXISTS public.service_escrows_booking_unique;

COMMIT;
