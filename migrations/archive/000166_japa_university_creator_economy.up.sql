-- Section 36 / ADR-063: Japa University Creator Economy
-- Adds the creator identity, revenue-split, and payout layer on top of the
-- existing dedicated learning schema (000039_learning, 000114_enrollments).
--
-- Reuses (not created here):
--   * enrollments  (000114) — course enrollment of record; +payment_id below
--   * reviews      (000019) — course reviews via entity_type='learning_track'
--   * payments/subscriptions (000022) — access + payout execution
--
-- All new columns are nullable/defaulted so existing rows and existing
-- callers keep working without changes.

-- =====================================================================
-- 1. Instructor profiles (one per user per tenant; gated approval)
-- =====================================================================
CREATE TABLE IF NOT EXISTS instructor_profiles (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id               uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    display_name          text NOT NULL,
    headline              text,
    bio                   text,
    social_links          jsonb NOT NULL DEFAULT '{}',   -- {youtube, instagram, blog, ...}
    tier                  text NOT NULL DEFAULT 'standard'
                            CHECK (tier IN ('standard','verified','partner')),
    status                text NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending','approved','rejected','suspended')),
    commission_pct_override numeric(5,2)                 -- NULL = use system default / tier default
                            CHECK (commission_pct_override IS NULL
                                   OR (commission_pct_override >= 0 AND commission_pct_override <= 100)),
    payout_method         jsonb,                          -- bank/transfer details; encrypted at app layer
    kyc_status            text NOT NULL DEFAULT 'unverified'
                            CHECK (kyc_status IN ('unverified','pending','verified','failed')),
    application_note      text,                           -- creator's pitch / content samples
    reviewed_by           uuid REFERENCES users(id),
    reviewed_at           timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_instructor_profiles_tenant_status
    ON instructor_profiles(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_instructor_profiles_user
    ON instructor_profiles(tenant_id, user_id);

COMMENT ON TABLE  instructor_profiles IS 'Japa University creator identity; gated application/approval (ADR-063).';
COMMENT ON COLUMN instructor_profiles.commission_pct_override IS 'Creator revenue % override; NULL falls back to tier/system default.';

-- =====================================================================
-- 2. Instructor payouts (one per instructor per payout period)
-- =====================================================================
CREATE TABLE IF NOT EXISTS instructor_payouts (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    instructor_id            uuid NOT NULL REFERENCES instructor_profiles(id) ON DELETE CASCADE,
    period_start             date NOT NULL,
    period_end               date NOT NULL,
    gross_kobo               bigint NOT NULL DEFAULT 0,
    platform_commission_kobo bigint NOT NULL DEFAULT 0,
    net_kobo                 bigint NOT NULL DEFAULT 0,
    currency                 text NOT NULL DEFAULT 'NGN',
    status                   text NOT NULL DEFAULT 'pending'
                               CHECK (status IN ('pending','processing','paid','failed')),
    payout_reference         text,                        -- provider transfer reference
    failure_reason           text,
    processed_at             timestamptz,
    created_at               timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, instructor_id, period_start, period_end)
);
CREATE INDEX IF NOT EXISTS idx_instructor_payouts_instructor
    ON instructor_payouts(tenant_id, instructor_id, period_start DESC);
CREATE INDEX IF NOT EXISTS idx_instructor_payouts_status
    ON instructor_payouts(tenant_id, status) WHERE status IN ('pending','processing');

COMMENT ON TABLE instructor_payouts IS 'Aggregated periodic creator payout; rolls up revenue_splits (ADR-063).';

-- =====================================================================
-- 3. Revenue splits (immutable per-event audit of how money was split)
-- =====================================================================
CREATE TABLE IF NOT EXISTS revenue_splits (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    instructor_id  uuid NOT NULL REFERENCES instructor_profiles(id) ON DELETE CASCADE,
    track_id       uuid NOT NULL REFERENCES learning_tracks(id) ON DELETE CASCADE,
    enrollment_id  uuid REFERENCES enrollments(id) ON DELETE SET NULL,
    payment_id     uuid REFERENCES payments(id) ON DELETE SET NULL,   -- null for pro_stream
    payout_id      uuid REFERENCES instructor_payouts(id) ON DELETE SET NULL, -- set when rolled up
    source         text NOT NULL DEFAULT 'purchase'
                     CHECK (source IN ('purchase','pro_stream')),
    gross_kobo     bigint NOT NULL DEFAULT 0,
    creator_pct    numeric(5,2) NOT NULL,
    platform_pct   numeric(5,2) NOT NULL,
    creator_kobo   bigint NOT NULL DEFAULT 0,
    platform_kobo  bigint NOT NULL DEFAULT 0,
    currency       text NOT NULL DEFAULT 'NGN',
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_revenue_splits_instructor_open
    ON revenue_splits(tenant_id, instructor_id) WHERE payout_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_revenue_splits_payout
    ON revenue_splits(tenant_id, payout_id);

COMMENT ON TABLE revenue_splits IS 'Immutable per-event revenue split record; aggregated into instructor_payouts (ADR-063).';

-- =====================================================================
-- 4. Extend learning_tracks for the marketplace
-- =====================================================================
ALTER TABLE learning_tracks
    ADD COLUMN IF NOT EXISTS instructor_id              uuid REFERENCES instructor_profiles(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS price_kobo                 bigint,
    ADD COLUMN IF NOT EXISTS currency                   text NOT NULL DEFAULT 'NGN',
    ADD COLUMN IF NOT EXISTS is_published_to_university  boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS category                   text,
    ADD COLUMN IF NOT EXISTS language                   text NOT NULL DEFAULT 'en',
    ADD COLUMN IF NOT EXISTS total_enrolled             int NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_learning_tracks_university
    ON learning_tracks(tenant_id, is_published_to_university, category)
    WHERE is_published_to_university = true;
CREATE INDEX IF NOT EXISTS idx_learning_tracks_instructor
    ON learning_tracks(tenant_id, instructor_id);

COMMENT ON COLUMN learning_tracks.instructor_id IS 'Owning creator; NULL = platform/staff-authored course (ADR-063).';
COMMENT ON COLUMN learning_tracks.price_kobo IS 'À la carte price; NULL = free or Pro-only.';
COMMENT ON COLUMN learning_tracks.is_published_to_university IS 'Marketplace visibility; distinct from is_published (draft/live).';

-- =====================================================================
-- 5. Link enrollments to their payment (paid enrollments only)
-- =====================================================================
ALTER TABLE enrollments
    ADD COLUMN IF NOT EXISTS payment_id uuid REFERENCES payments(id) ON DELETE SET NULL;

COMMENT ON COLUMN enrollments.payment_id IS 'Section 17 payment for paid enrollments; NULL for free/Pro/pro_stream (ADR-063).';
