-- Phase: Affiliate Enhancements — Author Revenue Sharing + Unregistered Link Detection
-- Ref: Sections 55, 56 — affiliate system enhancements

-- 1. Add author commission percentage to affiliate providers
ALTER TABLE affiliate_providers
    ADD COLUMN author_commission_pct numeric(5,2) NOT NULL DEFAULT 0;

COMMENT ON COLUMN affiliate_providers.author_commission_pct IS
    'Percentage of external affiliate commission shared with the content author who embedded the link. 0 = no sharing.';

-- 2. Table for tracking unregistered affiliate links found in content
CREATE TABLE content_affiliate_scan_results (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id          uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    url                 text NOT NULL,
    domain              text NOT NULL,
    matched_provider_id uuid REFERENCES affiliate_providers(id) ON DELETE SET NULL,
    detection_method    text NOT NULL CHECK (detection_method IN ('param_match', 'domain_match', 'manual')),
    action_taken        text NOT NULL DEFAULT 'flagged' CHECK (action_taken IN ('flagged', 'blocked', 'stripped', 'approved')),
    reviewed_by         uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at         timestamptz,
    notes               text,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_scan_results_unreviewed ON content_affiliate_scan_results (tenant_id, reviewed_at) WHERE reviewed_at IS NULL;
CREATE INDEX idx_scan_results_content ON content_affiliate_scan_results (content_id);
CREATE INDEX idx_scan_results_domain ON content_affiliate_scan_results (domain);

COMMENT ON TABLE content_affiliate_scan_results IS 'Tracks affiliate links found in content that are not registered in affiliate_providers. Used for moderation and revenue tracking.';

-- 3. Table for tracking external affiliate commissions earned per content
CREATE TABLE affiliate_content_commissions (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id          uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    provider_id         uuid NOT NULL REFERENCES affiliate_providers(id) ON DELETE CASCADE,
    author_user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    external_commission numeric(12,2) NOT NULL,
    author_share        numeric(12,2) NOT NULL,
    author_commission_pct numeric(5,2) NOT NULL,
    event_description   text,
    credited            boolean NOT NULL DEFAULT false,
    credited_at         timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_aff_comm_pending ON affiliate_content_commissions (credited, created_at) WHERE credited = false;
CREATE INDEX idx_aff_comm_author ON affiliate_content_commissions (author_user_id, created_at DESC);

COMMENT ON TABLE affiliate_content_commissions IS 'Tracks external affiliate commissions earned from content with affiliate links, and the author revenue share.';
