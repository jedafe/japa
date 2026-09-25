-- 000422_tenant_domain_unique.down.sql
ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_domain_unique;
