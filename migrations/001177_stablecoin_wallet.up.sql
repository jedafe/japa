-- 001177_stablecoin_wallet.up.sql
-- §128 Non-Custodial Stablecoin Wallets: member smart wallets (one per
-- member, Crossmint custody-free), the onchain activity mirror (render
-- cache — chain is truth), Squads 2-of-3 service escrow vaults, and the
-- platform treasury registry (hot/cold/revenue roles).
--
-- Ref: .jeos/sections/128-stablecoin-wallet-custody.md

BEGIN;

CREATE TABLE public.stablecoin_wallets (
    id                 uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id            uuid NOT NULL,
    chain              text DEFAULT 'solana' NOT NULL,
    address            text NOT NULL,
    locator            text NOT NULL,
    status             text DEFAULT 'provisioned' NOT NULL,
    recovery_methods   jsonb DEFAULT '[]' NOT NULL,
    delegated_signer   jsonb,
    created_at         timestamp with time zone DEFAULT now() NOT NULL,
    updated_at         timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT stablecoin_wallets_pkey PRIMARY KEY (id),
    CONSTRAINT stablecoin_wallets_user_unique UNIQUE (user_id),
    CONSTRAINT stablecoin_wallets_address_unique UNIQUE (address),
    CONSTRAINT stablecoin_wallets_status_check CHECK ((status = ANY (ARRAY['provisioned'::text, 'active'::text, 'suspended'::text]))),
    CONSTRAINT stablecoin_wallets_user_fk FOREIGN KEY (user_id) REFERENCES public.users(id)
);

CREATE TABLE public.stablecoin_activity (
    id                   uuid DEFAULT gen_random_uuid() NOT NULL,
    wallet_id            uuid NOT NULL,
    direction            text NOT NULL,
    kind                 text NOT NULL,
    asset                text NOT NULL,
    amount_minor         bigint NOT NULL,
    credits_amount       bigint NOT NULL,
    counterparty_user_id uuid,
    tx_signature         text,
    slot                 bigint,
    status               text DEFAULT 'pending' NOT NULL,
    occurred_at          timestamp with time zone NOT NULL,
    raw                  jsonb,
    CONSTRAINT stablecoin_activity_pkey PRIMARY KEY (id),
    CONSTRAINT stablecoin_activity_direction_check CHECK ((direction = ANY (ARRAY['in'::text, 'out'::text]))),
    CONSTRAINT stablecoin_activity_asset_check CHECK ((asset = ANY (ARRAY['usdc'::text, 'usdt'::text]))),
    CONSTRAINT stablecoin_activity_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'confirmed'::text, 'failed'::text]))),
    CONSTRAINT stablecoin_activity_tx_signature_unique UNIQUE (tx_signature),
    CONSTRAINT stablecoin_activity_wallet_fk FOREIGN KEY (wallet_id) REFERENCES public.stablecoin_wallets(id),
    CONSTRAINT stablecoin_activity_counterparty_fk FOREIGN KEY (counterparty_user_id) REFERENCES public.users(id)
);

CREATE INDEX idx_sc_activity_wallet ON public.stablecoin_activity (wallet_id, occurred_at DESC);
CREATE INDEX idx_sc_activity_counterparty ON public.stablecoin_activity (counterparty_user_id) WHERE counterparty_user_id IS NOT NULL;

CREATE TABLE public.service_escrows (
    id                    uuid DEFAULT gen_random_uuid() NOT NULL,
    booking_id            uuid NOT NULL,
    buyer_wallet_id       uuid NOT NULL,
    seller_wallet_id      uuid NOT NULL,
    vault_address         text NOT NULL,
    asset                 text NOT NULL,
    amount_minor          bigint NOT NULL,
    fee_minor             bigint NOT NULL,
    state                 text DEFAULT 'funding' NOT NULL,
    funded_sig            text,
    release_sig           text,
    confirmation_deadline timestamp with time zone,
    arbiter_decision      jsonb,
    created_at            timestamp with time zone DEFAULT now() NOT NULL,
    updated_at            timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT service_escrows_pkey PRIMARY KEY (id),
    CONSTRAINT service_escrows_vault_unique UNIQUE (vault_address),
    CONSTRAINT service_escrows_asset_check CHECK ((asset = ANY (ARRAY['usdc'::text, 'usdt'::text]))),
    CONSTRAINT service_escrows_state_check CHECK ((state = ANY (ARRAY['funding'::text, 'funded'::text, 'released'::text, 'refunded'::text, 'disputed'::text]))),
    CONSTRAINT service_escrows_booking_fk FOREIGN KEY (booking_id) REFERENCES public.service_bookings(id),
    CONSTRAINT service_escrows_buyer_wallet_fk FOREIGN KEY (buyer_wallet_id) REFERENCES public.stablecoin_wallets(id),
    CONSTRAINT service_escrows_seller_wallet_fk FOREIGN KEY (seller_wallet_id) REFERENCES public.stablecoin_wallets(id)
);

CREATE INDEX idx_service_escrows_booking ON public.service_escrows (booking_id);
CREATE INDEX idx_service_escrows_state ON public.service_escrows (state) WHERE state IN ('funding', 'funded', 'disputed');

CREATE TABLE public.treasury_wallets (
    id              uuid DEFAULT gen_random_uuid() NOT NULL,
    role            text NOT NULL,
    chain           text DEFAULT 'solana' NOT NULL,
    address         text NOT NULL,
    locator         text,
    float_cap_minor bigint,
    created_at      timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT treasury_wallets_pkey PRIMARY KEY (id),
    CONSTRAINT treasury_wallets_role_check CHECK ((role = ANY (ARRAY['hot'::text, 'cold'::text, 'revenue'::text]))),
    CONSTRAINT treasury_wallets_chain_role_unique UNIQUE (chain, role)
);

COMMIT;
