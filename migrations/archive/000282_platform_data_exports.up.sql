-- Platform Data Exports — admin-initiated exports of platform data
-- (users, posts, comments, orders, etc.) in CSV/JSON/XLSX format.
-- Separate from data_export_requests (Section 19 GDPR per-user exports).
-- Ref: japaconnect-engineering/sections/67-platform-data-portability.md

CREATE TABLE platform_data_exports (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            text NOT NULL,
    export_type     text NOT NULL,   -- 'users'|'posts'|'comments'|'orders'|'transactions'|'analytics'|'moderation'
    format          text NOT NULL DEFAULT 'csv',  -- 'csv'|'json'|'xlsx'
    status          text NOT NULL DEFAULT 'pending',  -- 'pending'|'processing'|'completed'|'failed'
    file_url        text,
    file_size       bigint,
    include_metadata boolean NOT NULL DEFAULT true,
    error_message   text,
    created_by      uuid NOT NULL REFERENCES users(id),
    completed_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_platform_data_exports_tenant ON platform_data_exports(tenant_id, created_at DESC);
CREATE INDEX idx_platform_data_exports_status ON platform_data_exports(status);
