-- 000002_create_tenants_table.sql
-- Create the tenants table — the root of multi-tenancy in JapaConnect.
-- Every scoped table carries a tenant_id that references this table.
-- The default tenant is seeded so the platform works out of the box.

CREATE TABLE tenants (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    slug        text NOT NULL UNIQUE,
    domain      text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Seed the default tenant with a fixed UUID so foreign keys can reference it
-- deterministically across environments.
INSERT INTO tenants (id, name, slug) VALUES (
    '00000000-0000-0000-0000-000000000001',
    'JapaConnect',
    'japaconnect'
);
