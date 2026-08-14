-- Down migration for 000002_create_tenants_table.sql
-- Drops the tenants table (seeded default tenant is removed with the table).
DROP TABLE IF EXISTS tenants;
