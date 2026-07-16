-- Add missing tenant_id to reaction_types and reactions tables.
-- These columns were defined in migration 000010 but are absent from the
-- live database, causing INSERT/UPDATE/SELECT to fail.

-- 1. Drop the existing PK (key only) and FK on reactions.reaction_key
ALTER TABLE reactions DROP CONSTRAINT IF EXISTS reactions_reaction_key_fkey;
ALTER TABLE reaction_types DROP CONSTRAINT IF EXISTS reaction_types_pkey;

-- 2. Add tenant_id to reaction_types (nullable first for backfill)
ALTER TABLE reaction_types ADD COLUMN IF NOT EXISTS tenant_id uuid;

-- 3. Backfill existing rows with the default tenant
UPDATE reaction_types SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;

-- 4. Make tenant_id NOT NULL and set the correct PK
ALTER TABLE reaction_types ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE reaction_types ADD CONSTRAINT reaction_types_tenant_fk
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE reaction_types ADD CONSTRAINT reaction_types_pkey
    PRIMARY KEY (tenant_id, key);

CREATE INDEX IF NOT EXISTS idx_reaction_types_tenant ON reaction_types(tenant_id);

-- 5. Add tenant_id to reactions
ALTER TABLE reactions ADD COLUMN IF NOT EXISTS tenant_id uuid;

UPDATE reactions SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;

ALTER TABLE reactions ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE reactions ADD CONSTRAINT reactions_tenant_fk
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE reactions DROP CONSTRAINT IF EXISTS reactions_pkey;
ALTER TABLE reactions ADD CONSTRAINT reactions_pkey
    PRIMARY KEY (tenant_id, entity_type, entity_id, user_id, reaction_key);

-- 6. Restore the FK from reactions to reaction_types (tenant-scoped)
ALTER TABLE reactions ADD CONSTRAINT reactions_reaction_key_fkey
    FOREIGN KEY (tenant_id, reaction_key) REFERENCES reaction_types(tenant_id, key) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_reactions_entity ON reactions(tenant_id, entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_reactions_user ON reactions(tenant_id, user_id);
