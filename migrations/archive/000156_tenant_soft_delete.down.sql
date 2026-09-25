DROP INDEX IF EXISTS idx_tenants_deleted_at;

ALTER TABLE tenants DROP CONSTRAINT tenants_status_check;
ALTER TABLE tenants ADD CONSTRAINT tenants_status_check CHECK (status IN ('active', 'suspended'));

ALTER TABLE tenants DROP COLUMN IF EXISTS deleted_at;
