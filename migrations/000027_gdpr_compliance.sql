-- Section 19: GDPR / NDPR Compliance

-- =====================
-- Consent Records
-- =====================

CREATE TABLE consent_records (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         uuid REFERENCES users(id) ON DELETE SET NULL,  -- NULL for anonymous
    consent_type    text NOT NULL,  -- 'marketing_email'|'analytics_tracking'|'third_party_sharing'|'ai_processing'
    granted         boolean NOT NULL,
    policy_version  text NOT NULL,
    ip_address      inet,
    user_agent      text,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_consent_records_user ON consent_records(user_id, consent_type);
CREATE INDEX idx_consent_records_tenant ON consent_records(tenant_id, consent_type);

-- =====================
-- Data Export Requests
-- =====================

CREATE TABLE data_export_requests (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id),
    status          text NOT NULL DEFAULT 'pending',  -- 'pending'|'processing'|'completed'|'failed'
    download_url    text,
    expires_at      timestamptz,
    completed_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_data_export_user ON data_export_requests(user_id, status);

-- =====================
-- Account Deletion Requests (soft-delete + anonymize)
-- =====================

CREATE TABLE account_deletion_requests (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id),
    status          text NOT NULL DEFAULT 'pending',  -- 'pending'|'processing'|'completed'|'failed'
    reason          text,
    completed_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_account_deletion_user ON account_deletion_requests(user_id, status);

-- =====================
-- Audit Log (canonical store for all platform actions)
-- =====================

CREATE TABLE audit_logs (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    actor_user_id       uuid REFERENCES users(id),
    impersonated_user_id uuid REFERENCES users(id),
    action_type         text NOT NULL,
    target_type         text,
    target_id           uuid,
    metadata            jsonb NOT NULL DEFAULT '{}',
    ip_address          inet,
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_tenant ON audit_logs(tenant_id, action_type, created_at DESC);
CREATE INDEX idx_audit_logs_actor ON audit_logs(actor_user_id, created_at DESC);
CREATE INDEX idx_audit_logs_target ON audit_logs(target_type, target_id) WHERE target_id IS NOT NULL;
