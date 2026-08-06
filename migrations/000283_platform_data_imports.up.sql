-- Platform Data Imports — admin-initiated imports of platform data
-- (users, posts, comments, orders, etc.) from CSV/JSON files.
-- Supports both migration/restore (from a JapaConnect export file) and
-- bulk seed data creation.
-- Ref: japaconnect-engineering/sections/67-platform-data-portability.md

CREATE TABLE platform_data_imports (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            text NOT NULL,
    import_type     text NOT NULL,   -- 'users'|'posts'|'comments'|'orders'|'transactions'|'analytics'|'moderation'
    format          text NOT NULL DEFAULT 'csv',  -- 'csv'|'json'
    status          text NOT NULL DEFAULT 'pending',  -- 'pending'|'processing'|'completed'|'failed'|'partial'
    file_url        text NOT NULL,   -- path to the uploaded file in SeaweedFS/local
    total_rows      integer NOT NULL DEFAULT 0,
    imported_rows   integer NOT NULL DEFAULT 0,
    failed_rows     integer NOT NULL DEFAULT 0,
    error_log       jsonb NOT NULL DEFAULT '[]'::jsonb,  -- array of {row, error}
    mode            text NOT NULL DEFAULT 'create',  -- 'create'|'upsert'  (create = skip existing, upsert = update or insert)
    created_by      uuid NOT NULL REFERENCES users(id),
    completed_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_platform_data_imports_tenant ON platform_data_imports(tenant_id, created_at DESC);
CREATE INDEX idx_platform_data_imports_status ON platform_data_imports(status);
