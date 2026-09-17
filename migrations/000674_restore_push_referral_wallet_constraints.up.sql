-- 000674_restore_push_referral_wallet_constraints.up.sql
-- Residual de-tenant cleanup following 000671/000672/000673:
--
-- 1. push_subscriptions is recorded as applied (000042) but the table is
--    absent — it was dropped outside the migration chain. Live routes
--    (POST /push/subscribe, GET/DELETE /me/push-tokens) hit
--    SavePushSubscription / ListPushSubscriptionsByUser / DeletePushSubscription
--    and fail. Recreate the de-tenant form (no tenant_id) with the original
--    UNIQUE(user_id, endpoint) so the ON CONFLICT (user_id, endpoint) upsert
--    has its arbiter.
--
-- 2. referrals: CreateReferral uses ON CONFLICT (referee_id) DO NOTHING —
--    "a user can only be referred once" — but no unique constraint ever
--    existed on referee_id, so the upsert has always failed with 42P10.
--    Add UNIQUE(referee_id); referee_id is NULLable (pending referrals may
--    not yet resolve to a user) and plain UNIQUE correctly treats NULLs as
--    non-conflicting.
--
-- 3. referral_programs and wallet_settings were singleton-per-tenant via
--    UNIQUE(tenant_id) (migrations 000056, 000023). De-tenant dropped that,
--    so concurrent inserts could create multiple config rows. A constant-key
--    unique index ((true)) restores single-row semantics.

BEGIN;

CREATE TABLE IF NOT EXISTS push_subscriptions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint   text NOT NULL,
    p256dh     text NOT NULL,
    auth_key   text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, endpoint)
);
CREATE INDEX IF NOT EXISTS idx_push_subscriptions_user
    ON push_subscriptions (user_id);

-- Dedupe referrals on referee_id (keep most recent) before adding the unique.
DELETE FROM referrals a
USING referrals b
WHERE a.referee_id IS NOT NULL
  AND a.referee_id = b.referee_id
  AND (a.created_at, a.id) < (b.created_at, b.id);

CREATE UNIQUE INDEX IF NOT EXISTS referrals_referee_id_key
    ON referrals (referee_id);

CREATE UNIQUE INDEX IF NOT EXISTS referral_programs_singleton
    ON referral_programs ((true));

CREATE UNIQUE INDEX IF NOT EXISTS wallet_settings_singleton
    ON wallet_settings ((true));

COMMIT;
