-- 000260_newsletter_v2.up.sql
-- Newsletter & Digest System V2 (Section 62, ADR-080).
-- Extends the existing newsletter_subscribers and newsletter_campaigns tables
-- with V2 columns, adds the newsletter_unsubscribe_tokens table, and seeds
-- newsletter.* system_settings + RBAC nodes.
--
-- Ref: japaconnect-engineering/sections/62-newsletter-digest.md
--      japaconnect-engineering/adr/ADR-080-newsletter-digest.md

BEGIN;

-- =====================================================================
-- Extend newsletter_subscribers with V2 columns.
-- =====================================================================
ALTER TABLE newsletter_subscribers ADD COLUMN IF NOT EXISTS digest_enabled   boolean NOT NULL DEFAULT true;
ALTER TABLE newsletter_subscribers ADD COLUMN IF NOT EXISTS campaign_enabled boolean NOT NULL DEFAULT true;
ALTER TABLE newsletter_subscribers ADD COLUMN IF NOT EXISTS subscribed_at    timestamptz NOT NULL DEFAULT now();
ALTER TABLE newsletter_subscribers ADD COLUMN IF NOT EXISTS unsubscribed_at  timestamptz;
ALTER TABLE newsletter_subscribers ADD COLUMN IF NOT EXISTS source           text NOT NULL DEFAULT 'signup'
    CHECK (source IN ('signup','onboarding','footer','import','api'));

-- Extend the status CHECK to include 'bounced' and 'complained'.
ALTER TABLE newsletter_subscribers DROP CONSTRAINT IF EXISTS newsletter_subscribers_status_check;
ALTER TABLE newsletter_subscribers ADD CONSTRAINT newsletter_subscribers_status_check
    CHECK (status IN ('subscribed','unsubscribed','bounced','complained'));

CREATE INDEX IF NOT EXISTS idx_newsletter_sub_status ON newsletter_subscribers (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_newsletter_sub_user ON newsletter_subscribers (user_id) WHERE user_id IS NOT NULL;

-- =====================================================================
-- Extend newsletter_campaigns with V2 columns.
-- =====================================================================
ALTER TABLE newsletter_campaigns ADD COLUMN IF NOT EXISTS preview_text   text;
ALTER TABLE newsletter_campaigns ADD COLUMN IF NOT EXISTS body           jsonb NOT NULL DEFAULT '{}';
ALTER TABLE newsletter_campaigns ADD COLUMN IF NOT EXISTS segment        text NOT NULL DEFAULT 'all'
    CHECK (segment IN ('all','space','tag','pro_subscribers'));
ALTER TABLE newsletter_campaigns ADD COLUMN IF NOT EXISTS segment_id     uuid;
ALTER TABLE newsletter_campaigns ADD COLUMN IF NOT EXISTS scheduled_at   timestamptz;
ALTER TABLE newsletter_campaigns ADD COLUMN IF NOT EXISTS recipient_count int NOT NULL DEFAULT 0;
ALTER TABLE newsletter_campaigns ADD COLUMN IF NOT EXISTS open_count     int NOT NULL DEFAULT 0;
ALTER TABLE newsletter_campaigns ADD COLUMN IF NOT EXISTS click_count    int NOT NULL DEFAULT 0;
ALTER TABLE newsletter_campaigns ADD COLUMN IF NOT EXISTS created_by     uuid REFERENCES users(id);

-- Extend the status CHECK to include 'scheduled' and 'cancelled'.
ALTER TABLE newsletter_campaigns DROP CONSTRAINT IF EXISTS newsletter_campaigns_status_check;
ALTER TABLE newsletter_campaigns ADD CONSTRAINT newsletter_campaigns_status_check
    CHECK (status IN ('draft','scheduled','sending','sent','cancelled'));

-- =====================================================================
-- newsletter_unsubscribe_tokens — one-click unsubscribe tokens.
-- =====================================================================
CREATE TABLE IF NOT EXISTS newsletter_unsubscribe_tokens (
    token         text PRIMARY KEY,
    subscriber_id uuid NOT NULL REFERENCES newsletter_subscribers(id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_unsub_tokens_subscriber ON newsletter_unsubscribe_tokens (subscriber_id);

-- =====================================================================
-- system_settings — 5 newsletter.* keys (Section 62, ADR-080)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'newsletter.enabled',        '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'newsletter.digest_enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'newsletter.digest_day',     '"1"'),
    ('00000000-0000-0000-0000-000000000001', 'newsletter.digest_hour_utc', '"8"'),
    ('00000000-0000-0000-0000-000000000001', 'newsletter.batch_size',     '"500"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 3 newsletter.* nodes (Section 62, ADR-080)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('newsletter.subscribe', 'Subscribe/unsubscribe own email'),
    ('newsletter.manage',    'Admin: create/send campaigns, view stats'),
    ('newsletter.import',    'Admin: import external subscriber list')
ON CONFLICT (key) DO NOTHING;

-- Grant newsletter.manage + newsletter.import to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('newsletter.manage', 'newsletter.import')
ON CONFLICT DO NOTHING;

-- Grant newsletter.subscribe to the default 'member' role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'newsletter.subscribe'
ON CONFLICT DO NOTHING;

COMMIT;
