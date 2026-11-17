-- 001182_escrow_booking_unique.up.sql
-- §128.5: one escrow per booking at the DB level. PrepareEscrow dedupes
-- via GetEscrowByBooking, but a concurrent double-prepare races the check
-- — the vault placeholder ('pending:<booking>') already collides on the
-- second insert, making the intent explicit rather than accidental.
--
-- Ref: .jeos/sections/128-stablecoin-wallet-custody.md

BEGIN;

CREATE UNIQUE INDEX service_escrows_booking_unique
    ON public.service_escrows (booking_id);

COMMIT;
