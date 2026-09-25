-- Control Panel: tenant soft-delete (catalog: DELETE /control/tenants/:id).
-- Section 21 specifies soft-delete with status='deleted' and a 30-day grace
-- period before the tenant:cleanup_data Asynq task hard-deletes data.
-- The existing hard DELETE FROM tenants fails with FK violations when the
-- tenant has any child rows in tables lacking ON DELETE CASCADE (users,
-- organizations, learning tables, webhooks, API keys, etc.).

ALTER TABLE tenants ADD COLUMN deleted_at timestamptz;

ALTER TABLE tenants DROP CONSTRAINT tenants_status_check;
ALTER TABLE tenants ADD CONSTRAINT tenants_status_check CHECK (status IN ('active', 'suspended', 'deleted'));

CREATE INDEX idx_tenants_deleted_at ON tenants(deleted_at) WHERE deleted_at IS NOT NULL;
