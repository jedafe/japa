-- 000422_tenant_domain_unique.up.sql
-- Section 04 (Multi-Tenancy) specifies `domain text UNIQUE` but the original
-- migration (000002) created the column without a unique constraint. Without
-- it, two tenants can share the same domain, making GetTenantByHost ambiguous
-- (LIMIT 1 picks an arbitrary row) and risking cross-tenant routing.
-- PostgreSQL allows multiple NULLs in a UNIQUE column, so the default tenant
-- (seeded with domain = NULL) is unaffected.
ALTER TABLE tenants ADD CONSTRAINT tenants_domain_unique UNIQUE (domain);
