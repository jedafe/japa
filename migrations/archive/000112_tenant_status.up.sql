-- Control Panel: tenant suspend/unsuspend (catalog: POST /control/tenants/:id/suspend|unsuspend).
-- No status concept existed on tenants at all before this.
ALTER TABLE tenants ADD COLUMN status text NOT NULL DEFAULT 'active';
ALTER TABLE tenants ADD CONSTRAINT tenants_status_check CHECK (status IN ('active', 'suspended'));
CREATE INDEX idx_tenants_status ON tenants(status);
