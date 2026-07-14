-- Revert tenant_id addition to reaction_types and reactions.

ALTER TABLE reactions DROP CONSTRAINT IF EXISTS reactions_reaction_key_fkey;
ALTER TABLE reactions DROP CONSTRAINT IF EXISTS reactions_pkey;
ALTER TABLE reactions DROP CONSTRAINT IF EXISTS reactions_tenant_fk;
DROP INDEX IF EXISTS idx_reactions_entity;
DROP INDEX IF EXISTS idx_reactions_user;
ALTER TABLE reactions DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE reactions ADD CONSTRAINT reactions_pkey
    PRIMARY KEY (entity_type, entity_id, user_id, reaction_key);

ALTER TABLE reaction_types DROP CONSTRAINT IF EXISTS reaction_types_pkey;
ALTER TABLE reaction_types DROP CONSTRAINT IF EXISTS reaction_types_tenant_fk;
DROP INDEX IF EXISTS idx_reaction_types_tenant;
ALTER TABLE reaction_types DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE reaction_types ADD CONSTRAINT reaction_types_pkey
    PRIMARY KEY (key);

ALTER TABLE reactions ADD CONSTRAINT reactions_reaction_key_fkey
    FOREIGN KEY (reaction_key) REFERENCES reaction_types(key) ON DELETE CASCADE;
