-- Section 33 (Newsletter): Newsletter subscribers and campaigns tables
-- Supports subscriber management and campaign creation for newsletters.

CREATE TABLE IF NOT EXISTS newsletter_subscribers (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email      text NOT NULL,
    user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    status     text NOT NULL DEFAULT 'active',  -- 'active' | 'unsubscribed' | 'bounced'
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, email)
);
CREATE INDEX idx_newsletter_subscribers_tenant ON newsletter_subscribers(tenant_id, status);

CREATE TABLE IF NOT EXISTS newsletter_campaigns (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name       text NOT NULL,
    subject    text NOT NULL,
    html       text NOT NULL DEFAULT '',
    status     text NOT NULL DEFAULT 'draft',  -- 'draft' | 'scheduled' | 'sent'
    sent_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_newsletter_campaigns_tenant ON newsletter_campaigns(tenant_id, status);
