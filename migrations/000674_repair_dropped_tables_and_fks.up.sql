-- 000674_repair_dropped_tables_and_fks.up.sql
--
-- Repairs two classes of schema damage found during the scholarship audit:
--
-- 1. Legacy sql-migrate-style files with an embedded "-- +migrate Down"
--    section (000042, 000044, 000052). The forward-only migrator executes
--    whole files and does not split on those annotations, so each of these
--    migrations created its tables and immediately dropped them in the same
--    transaction. On any database that ran them, the following tables are
--    missing entirely: push_subscriptions, newsletter_lists,
--    newsletter_list_subscribers, mentorship_profiles, mentor_sessions,
--    mentor_reviews, ads_campaigns, bookings, booking_slots,
--    external_data_sources, content_engagement. This migration recreates
--    them in their current de-tenant form (tenant_id was dropped platform-
--    wide by 000668 and the tenants table no longer exists, so the original
--    tenant-scoped DDL cannot be replayed verbatim).
--
-- 2. referrals had no UNIQUE on referee_id in any migration, so the
--    CreateReferral query's ON CONFLICT (referee_id) always failed with
--    SQLSTATE 42P10. Adds the unique index after deduping.
--
-- 3. reactions.reaction_key originally referenced reaction_types via the
--    composite FK (tenant_id, reaction_key) -> (tenant_id, key). When
--    tenant_id was dropped the FK went with it; reaction_types' PK is now
--    (key), so the de-tenant single-column FK is restored.
--
-- Ref: 000042/000044/000052 (embedded-down defect), 000668 (de-tenant),
--      000670-000673 (per-table constraint restores).

BEGIN;

-- ---------------------------------------------------------------------------
-- 1. Recreate tables dropped by embedded "-- +migrate Down" sections.
-- ---------------------------------------------------------------------------

-- 000042: web push notification subscriptions (SavePushSubscription upserts
-- on (user_id, endpoint)).
CREATE TABLE IF NOT EXISTS push_subscriptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint text NOT NULL,
    p256dh text NOT NULL,
    auth_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(user_id, endpoint)
);
CREATE INDEX IF NOT EXISTS idx_push_subscriptions_user ON push_subscriptions(user_id);

-- 000044: newsletter lists (de-tenant: UNIQUE(tenant_id, name) -> UNIQUE(name)).
CREATE TABLE IF NOT EXISTS newsletter_lists (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    description text,
    is_default boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(name)
);

-- 000044: newsletter_subscribers is normally still present (recreated by
-- 000051 after 000044 self-dropped it), so this is a safety-net no-op where
-- it exists. De-tenant form; UNIQUE(email) matches the restored index in
-- 000671 (newsletter_subscribers_tenant_id_email_key).
CREATE TABLE IF NOT EXISTS newsletter_subscribers (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email      text NOT NULL,
    user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    status     text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(email)
);

-- 000044: newsletter list <-> subscriber junction.
CREATE TABLE IF NOT EXISTS newsletter_list_subscribers (
    list_id uuid NOT NULL REFERENCES newsletter_lists(id) ON DELETE CASCADE,
    subscriber_id uuid NOT NULL REFERENCES newsletter_subscribers(id) ON DELETE CASCADE,
    PRIMARY KEY (list_id, subscriber_id)
);

-- 000044: mentorship profiles (de-tenant: UNIQUE(tenant_id, user_id) ->
-- UNIQUE(user_id)).
CREATE TABLE IF NOT EXISTS mentorship_profiles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bio text,
    skills jsonb NOT NULL DEFAULT '[]',
    goals jsonb NOT NULL DEFAULT '[]',
    is_mentor boolean NOT NULL DEFAULT true,
    is_available boolean NOT NULL DEFAULT true,
    rating_avg numeric(3,2) DEFAULT 0,
    session_count int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(user_id)
);
CREATE INDEX IF NOT EXISTS idx_mentorship_profiles_user ON mentorship_profiles(user_id);
CREATE INDEX IF NOT EXISTS idx_mentorship_profiles_mentor ON mentorship_profiles(is_mentor);
CREATE INDEX IF NOT EXISTS idx_mentorship_profiles_available ON mentorship_profiles(is_available);

-- 000044: mentor sessions.
CREATE TABLE IF NOT EXISTS mentor_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    mentor_id uuid NOT NULL REFERENCES mentorship_profiles(id) ON DELETE CASCADE,
    mentee_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'scheduled',
    scheduled_at timestamptz NOT NULL,
    started_at timestamptz,
    completed_at timestamptz,
    notes text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mentor_sessions_mentor ON mentor_sessions(mentor_id);
CREATE INDEX IF NOT EXISTS idx_mentor_sessions_mentee ON mentor_sessions(mentee_id);
CREATE INDEX IF NOT EXISTS idx_mentor_sessions_status ON mentor_sessions(status);

-- 000044: mentor reviews (no tenant_id in the original DDL).
CREATE TABLE IF NOT EXISTS mentor_reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES mentor_sessions(id) ON DELETE CASCADE,
    reviewer_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating int NOT NULL CHECK (rating >= 1 AND rating <= 5),
    comment text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(session_id, reviewer_id)
);
CREATE INDEX IF NOT EXISTS idx_mentor_reviews_session ON mentor_reviews(session_id);
CREATE INDEX IF NOT EXISTS idx_mentor_reviews_reviewer ON mentor_reviews(reviewer_id);

-- 000044: ads campaigns.
CREATE TABLE IF NOT EXISTS ads_campaigns (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    advertiser_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL,
    placement text NOT NULL,
    budget_credits int NOT NULL DEFAULT 0,
    spent_credits int NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'draft',
    start_date timestamptz,
    end_date timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ads_campaigns_advertiser ON ads_campaigns(advertiser_id);
CREATE INDEX IF NOT EXISTS idx_ads_campaigns_status ON ads_campaigns(status);

-- 000044: bookings.
CREATE TABLE IF NOT EXISTS bookings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id uuid NOT NULL,
    booker_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    slot_start timestamptz NOT NULL,
    slot_end timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    deposit_paid boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_bookings_service ON bookings(service_id);
CREATE INDEX IF NOT EXISTS idx_bookings_booker ON bookings(booker_id);
CREATE INDEX IF NOT EXISTS idx_bookings_status ON bookings(status);

-- 000044: booking slots.
CREATE TABLE IF NOT EXISTS booking_slots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id uuid NOT NULL,
    start_time timestamptz NOT NULL,
    end_time timestamptz NOT NULL,
    is_available boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_booking_slots_service ON booking_slots(service_id);
CREATE INDEX IF NOT EXISTS idx_booking_slots_available ON booking_slots(is_available);

-- 000044: external data sources (de-tenant: UNIQUE(tenant_id, name) ->
-- UNIQUE(name)).
CREATE TABLE IF NOT EXISTS external_data_sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    source_type text NOT NULL,
    api_endpoint text,
    sync_frequency text NOT NULL DEFAULT 'daily',
    last_synced_at timestamptz,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(name)
);
CREATE INDEX IF NOT EXISTS idx_external_data_sources_type ON external_data_sources(source_type);
CREATE INDEX IF NOT EXISTS idx_external_data_sources_status ON external_data_sources(status);

-- 000052: content engagement tracking (velocity scoring, Section 11).
CREATE TABLE IF NOT EXISTS content_engagement (
    content_id      UUID PRIMARY KEY REFERENCES contents(id) ON DELETE CASCADE,
    views           INT NOT NULL DEFAULT 0,
    comments_count  INT NOT NULL DEFAULT 0,
    votes_score     INT NOT NULL DEFAULT 0,
    bookmarks_count INT NOT NULL DEFAULT 0,
    shares_count    INT NOT NULL DEFAULT 0,
    velocity_score  FLOAT NOT NULL DEFAULT 0,
    last_calculated_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_content_engagement_velocity
    ON content_engagement(velocity_score DESC);

-- ---------------------------------------------------------------------------
-- 2. referrals.referee_id unique — required by CreateReferral's
--    ON CONFLICT (referee_id). A user can only be referred once.
-- ---------------------------------------------------------------------------
DELETE FROM referrals a USING referrals b
WHERE a.referee_id IS NOT DISTINCT FROM b.referee_id
  AND a.referee_id IS NOT NULL
  AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS referrals_referee_id_key ON referrals (referee_id);

-- ---------------------------------------------------------------------------
-- 3. Restore the de-tenant FK reactions.reaction_key -> reaction_types.key
--    (was (tenant_id, reaction_key) -> (tenant_id, key) before 000668).
--    reaction_types' primary key is now (key), so the single-column FK is
--    valid. Guard: drop any orphan reaction rows that reference a missing
--    reaction_type key before adding the constraint.
-- ---------------------------------------------------------------------------
DELETE FROM reactions r
WHERE NOT EXISTS (SELECT 1 FROM reaction_types rt WHERE rt.key = r.reaction_key);
ALTER TABLE reactions
    ADD CONSTRAINT reactions_reaction_key_fkey
    FOREIGN KEY (reaction_key) REFERENCES reaction_types(key) ON DELETE CASCADE;

COMMIT;
