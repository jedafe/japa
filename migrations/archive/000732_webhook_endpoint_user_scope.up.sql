-- Phase 3 (spec 2026-09-24-agent-data-api-phase3): personal webhook
-- subscriptions owned by an API-key holder. scope discriminates the
-- existing admin-managed platform endpoints from user-owned rows; the
-- owner is carried by the existing created_by column (FK users,
-- ON DELETE SET NULL from migration 000361).
ALTER TABLE webhook_endpoints
    ADD COLUMN scope text NOT NULL DEFAULT 'platform'
        CHECK (scope IN ('platform', 'user'));

CREATE INDEX idx_webhook_endpoints_user
    ON webhook_endpoints (created_by) WHERE scope = 'user';
