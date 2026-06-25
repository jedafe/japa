-- Feature flags table for pluggable architecture.
-- Every non-core platform feature is toggleable by admin.
-- When disabled: API returns 404, nav items hidden, no errors.
CREATE TABLE feature_flags (
    key          text NOT NULL,
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    enabled      boolean NOT NULL DEFAULT true,
    config       jsonb NOT NULL DEFAULT '{}',
    updated_at   timestamptz NOT NULL DEFAULT now(),
    updated_by   uuid REFERENCES users(id),
    PRIMARY KEY (key, tenant_id)
);

CREATE INDEX idx_feature_flags_tenant ON feature_flags(tenant_id);

-- Seed default feature flags for the default tenant
-- Core features (auth, RBAC, basic content) are NOT feature-flagged - they always exist
INSERT INTO feature_flags (key, tenant_id, enabled) VALUES
    -- Content features
    ('feature.blog',             '00000000-0000-0000-0000-000000000001', true),
    ('feature.forum',            '00000000-0000-0000-0000-000000000001', true),
    ('feature.comparison',       '00000000-0000-0000-0000-000000000001', true),
    ('feature.learning',         '00000000-0000-0000-0000-000000000001', false),
    -- Social features
    ('feature.follow',           '00000000-0000-0000-0000-000000000001', true),
    ('feature.dm',               '00000000-0000-0000-0000-000000000001', true),
    ('feature.reactions',        '00000000-0000-0000-0000-000000000001', true),
    ('feature.bookmarks',        '00000000-0000-0000-0000-000000000001', true),
    ('feature.ratings_reviews',  '00000000-0000-0000-0000-000000000001', true),
    ('feature.mentorship',       '00000000-0000-0000-0000-000000000001', false),
    -- Commerce features
    ('feature.wallet',           '00000000-0000-0000-0000-000000000001', false),
    ('feature.ads',              '00000000-0000-0000-0000-000000000001', false),
    ('feature.referral',         '00000000-0000-0000-0000-000000000001', false),
    -- Tools
    ('feature.tickets',          '00000000-0000-0000-0000-000000000001', true),
    ('feature.newsletter',       '00000000-0000-0000-0000-000000000001', false),
    ('feature.notifications',    '00000000-0000-0000-0000-000000000001', true),
    -- AI features
    ('feature.japabot',          '00000000-0000-0000-0000-000000000001', false),
    ('feature.ai_content_gen',   '00000000-0000-0000-0000-000000000001', false);
