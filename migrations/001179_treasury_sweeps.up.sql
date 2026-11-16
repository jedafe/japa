-- 001179_treasury_sweeps.up.sql
-- §128.7: hot→cold sweep proposal queue. The Squads ceremony
-- (propose → co-sign → execute) happens off-backend; this table records
-- the proposal, the treasury.sweep_approve decision, and the executed
-- signature for reconciliation.
--
-- Ref: .jeos/sections/128-stablecoin-wallet-custody.md

BEGIN;

CREATE TABLE public.treasury_sweeps (
    id                 uuid DEFAULT gen_random_uuid() NOT NULL,
    chain              text DEFAULT 'solana' NOT NULL,
    asset              text NOT NULL,
    amount_minor       bigint NOT NULL,
    hot_wallet_id      uuid NOT NULL,
    cold_address       text NOT NULL,
    state              text DEFAULT 'proposed' NOT NULL,
    proposed_by        text NOT NULL,
    approved_by        uuid,
    squads_proposal_id text,
    executed_sig       text,
    created_at         timestamp with time zone DEFAULT now() NOT NULL,
    decided_at         timestamp with time zone,
    executed_at        timestamp with time zone,
    CONSTRAINT treasury_sweeps_pkey PRIMARY KEY (id),
    CONSTRAINT treasury_sweeps_asset_check CHECK ((asset = ANY (ARRAY['usdc'::text, 'usdt'::text]))),
    CONSTRAINT treasury_sweeps_state_check CHECK ((state = ANY (ARRAY['proposed'::text, 'approved'::text, 'executed'::text, 'rejected'::text, 'expired'::text]))),
    CONSTRAINT treasury_sweeps_hot_wallet_fk FOREIGN KEY (hot_wallet_id) REFERENCES public.treasury_wallets(id),
    CONSTRAINT treasury_sweeps_approved_by_fk FOREIGN KEY (approved_by) REFERENCES public.users(id)
);

CREATE INDEX idx_treasury_sweeps_state ON public.treasury_sweeps (state, created_at DESC);

COMMIT;
