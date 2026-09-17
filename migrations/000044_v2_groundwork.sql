-- +migrate Up

-- Newsletter lists
CREATE TABLE IF NOT EXISTS newsletter_lists (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text,
    is_default boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_newsletter_lists_tenant ON newsletter_lists(tenant_id);

-- Newsletter subscribers (forward-created here, ahead of its home migration
-- 000051_newsletter.sql, because newsletter_list_subscribers below needs the
-- FK target to exist first. Column-for-column identical to 000051's
-- definition so that migration's own `CREATE TABLE IF NOT EXISTS` is a
-- true no-op on a database that ran this one first. Pre-existing ordering
-- defect discovered while building the ADR-061 integration-test harness
-- (Task A0): replaying every forward migration against a truly fresh
-- database is the first thing that exercises this path end-to-end.
CREATE TABLE IF NOT EXISTS newsletter_subscribers (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email      text NOT NULL,
    user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    status     text NOT NULL DEFAULT 'active',  -- 'active' | 'unsubscribed' | 'bounced'
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, email)
);

-- Newsletter list ↔ subscriber junction
CREATE TABLE IF NOT EXISTS newsletter_list_subscribers (
    list_id uuid NOT NULL REFERENCES newsletter_lists(id) ON DELETE CASCADE,
    subscriber_id uuid NOT NULL REFERENCES newsletter_subscribers(id) ON DELETE CASCADE,
    PRIMARY KEY (list_id, subscriber_id)
);

-- Mentorship profiles
CREATE TABLE IF NOT EXISTS mentorship_profiles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
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
    UNIQUE(tenant_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_mentorship_profiles_tenant ON mentorship_profiles(tenant_id);
CREATE INDEX IF NOT EXISTS idx_mentorship_profiles_user ON mentorship_profiles(user_id);
CREATE INDEX IF NOT EXISTS idx_mentorship_profiles_mentor ON mentorship_profiles(is_mentor);
CREATE INDEX IF NOT EXISTS idx_mentorship_profiles_available ON mentorship_profiles(is_available);

-- Mentor sessions
CREATE TABLE IF NOT EXISTS mentor_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    mentor_id uuid NOT NULL REFERENCES mentorship_profiles(id) ON DELETE CASCADE,
    mentee_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'scheduled',
    scheduled_at timestamptz NOT NULL,
    started_at timestamptz,
    completed_at timestamptz,
    notes text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mentor_sessions_tenant ON mentor_sessions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_mentor_sessions_mentor ON mentor_sessions(mentor_id);
CREATE INDEX IF NOT EXISTS idx_mentor_sessions_mentee ON mentor_sessions(mentee_id);
CREATE INDEX IF NOT EXISTS idx_mentor_sessions_status ON mentor_sessions(status);

-- Mentor reviews
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

-- Ads campaigns
CREATE TABLE IF NOT EXISTS ads_campaigns (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
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
CREATE INDEX IF NOT EXISTS idx_ads_campaigns_tenant ON ads_campaigns(tenant_id);
CREATE INDEX IF NOT EXISTS idx_ads_campaigns_advertiser ON ads_campaigns(advertiser_id);
CREATE INDEX IF NOT EXISTS idx_ads_campaigns_status ON ads_campaigns(status);

-- Bookings
CREATE TABLE IF NOT EXISTS bookings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    service_id uuid NOT NULL,
    booker_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    slot_start timestamptz NOT NULL,
    slot_end timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    deposit_paid boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_bookings_tenant ON bookings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_bookings_service ON bookings(service_id);
CREATE INDEX IF NOT EXISTS idx_bookings_booker ON bookings(booker_id);
CREATE INDEX IF NOT EXISTS idx_bookings_status ON bookings(status);

-- Booking slots
CREATE TABLE IF NOT EXISTS booking_slots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    service_id uuid NOT NULL,
    start_time timestamptz NOT NULL,
    end_time timestamptz NOT NULL,
    is_available boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_booking_slots_tenant ON booking_slots(tenant_id);
CREATE INDEX IF NOT EXISTS idx_booking_slots_service ON booking_slots(service_id);
CREATE INDEX IF NOT EXISTS idx_booking_slots_available ON booking_slots(is_available);

-- External data sources (Travel Comparison V3)
CREATE TABLE IF NOT EXISTS external_data_sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    source_type text NOT NULL,
    api_endpoint text,
    sync_frequency text NOT NULL DEFAULT 'daily',
    last_synced_at timestamptz,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_external_data_sources_tenant ON external_data_sources(tenant_id);
CREATE INDEX IF NOT EXISTS idx_external_data_sources_type ON external_data_sources(source_type);
CREATE INDEX IF NOT EXISTS idx_external_data_sources_status ON external_data_sources(status);
