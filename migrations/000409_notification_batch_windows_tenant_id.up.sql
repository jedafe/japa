-- Section 35: Add tenant_id to notification_batch_windows.
-- BUG-001 fix: batch dispatch was inserting notifications with a zero tenant_id
-- because the batch window table had no tenant context. This adds the column
-- (NOT NULL, backfilled to the default tenant for existing rows) and an index
-- for tenant-scoped cleanup.
-- Ref: japaconnect-engineering/sections/35-notification-system.md

ALTER TABLE notification_batch_windows
    ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001' REFERENCES tenants(id) ON DELETE CASCADE;

-- Drop the default so future inserts must supply the tenant explicitly.
ALTER TABLE notification_batch_windows ALTER COLUMN tenant_id DROP DEFAULT;

CREATE INDEX idx_notification_batch_windows_tenant
    ON notification_batch_windows (tenant_id, recipient_id, dispatched, window_close_at);
