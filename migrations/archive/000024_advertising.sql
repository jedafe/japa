-- Section 25: Ad Placement
-- Ad slot types, campaigns, event ingestion, analytics, and organization boosts.

-- =====================
-- Ad Slot Types (static registry — populated via migration seed)
-- =====================
CREATE TABLE ad_slot_types (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slot_name           text NOT NULL,
    description         text,
    max_display_per_page int NOT NULL DEFAULT 1,
    is_active           boolean NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slot_name)
);
CREATE INDEX idx_ad_slot_types_tenant ON ad_slot_types(tenant_id);
CREATE INDEX idx_ad_slot_types_name ON ad_slot_types(tenant_id, slot_name);

-- Seed the five predefined slot types for the default tenant.
-- Default tenant ID is inserted at deployment; this migration uses a placeholder
-- that gets replaced by the admin seeding scripts.
--
-- INSERT INTO ad_slot_types (tenant_id, slot_name, description, max_display_per_page) VALUES
--     (@default_tenant_id, 'feed_inline',           'In-feed content ad between posts',     ............2),
--     (@default_tenant_id, 'sidebar',               'Sidebar display ad',                    ............1),
--     (@default_tenant_id, 'search_sponsored',     'Sponsored result in search listings',   ............3),
--     (@default_tenant_id, 'comparison_sponsored',  'Featured row in comparison tables',     ............2),
--     (@default_tenant_id, 'forum_inline',          'In-forum ad between thread posts',      ............2);

-- =====================
-- Ad Campaigns
-- =====================
CREATE TABLE ad_campaigns (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name                text NOT NULL,
    slot_type_id        uuid NOT NULL REFERENCES ad_slot_types(id),
    content             jsonb NOT NULL DEFAULT '{}',    -- creative / ad content payload
    target_url          text,                            -- click-through destination
    budget              bigint,                          -- total campaign budget in kobo/cents, null = unlimited
    spent               bigint NOT NULL DEFAULT 0,       -- total spent so far
    status              text NOT NULL DEFAULT 'draft',   -- draft|active|paused|ended|rejected
    priority            int NOT NULL DEFAULT 0,          -- higher = served first when multiple campaigns match
    frequency_cap_per_user int,                          -- max impressions per user per day, null = no cap
    start_date          timestamptz,                     -- null = starts immediately when activated
    end_date            timestamptz,                     -- null = runs indefinitely
    created_by          uuid REFERENCES users(id),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ad_campaigns_tenant ON ad_campaigns(tenant_id);
CREATE INDEX idx_ad_campaigns_slot ON ad_campaigns(tenant_id, slot_type_id, status);
CREATE INDEX idx_ad_campaigns_status ON ad_campaigns(status);
CREATE INDEX idx_ad_campaigns_dates ON ad_campaigns(start_date, end_date);

-- =====================
-- Ad Events (batched impression/click ingestion)
-- =====================
CREATE TABLE ad_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    campaign_id uuid NOT NULL REFERENCES ad_campaigns(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type  text NOT NULL CHECK (event_type IN ('impression', 'click')),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ad_events_campaign ON ad_events(campaign_id, event_type);
CREATE INDEX idx_ad_events_user ON ad_events(user_id, event_type);
CREATE INDEX idx_ad_events_tenant ON ad_events(tenant_id);
-- Index for frequency capping: count impressions per user per campaign per day
CREATE INDEX idx_ad_events_user_campaign_impression
    ON ad_events(user_id, campaign_id, occurred_at)
    WHERE event_type = 'impression';

-- =====================
-- Ad Analytics Daily (pre-aggregated for dashboards)
-- =====================
CREATE TABLE ad_analytics_daily (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    campaign_id uuid NOT NULL REFERENCES ad_campaigns(id) ON DELETE CASCADE,
    slot_type_id uuid NOT NULL REFERENCES ad_slot_types(id),
    date        date NOT NULL,
    impressions int NOT NULL DEFAULT 0,
    clicks      int NOT NULL DEFAULT 0,
    spent       bigint NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, campaign_id, slot_type_id, date)
);
CREATE INDEX idx_ad_analytics_campaign_date ON ad_analytics_daily(campaign_id, date);
CREATE INDEX idx_ad_analytics_tenant ON ad_analytics_daily(tenant_id);

-- =====================
-- Organization Boosts (uses same campaign infrastructure)
-- =====================
CREATE TABLE organization_boosts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    campaign_id     uuid NOT NULL REFERENCES ad_campaigns(id) ON DELETE CASCADE,
    boost_type      text NOT NULL DEFAULT 'featured_placement',
    status          text NOT NULL DEFAULT 'active',   -- active|ended|canceled
    start_date      timestamptz NOT NULL DEFAULT now(),
    end_date        timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_organization_boosts_org ON organization_boosts(organization_id);
CREATE INDEX idx_organization_boosts_campaign ON organization_boosts(campaign_id);
CREATE INDEX idx_organization_boosts_tenant ON organization_boosts(tenant_id);