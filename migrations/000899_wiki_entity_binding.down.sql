-- 000899_wiki_entity_binding.down.sql
-- Reverses 000899: drops the entity binding on wiki_pages, the entity-kind
-- columns on wiki_type_configs, and the wiki_entity_kinds registry.

BEGIN;

DROP INDEX IF EXISTS idx_wiki_pages_entity;
DROP INDEX IF EXISTS wiki_pages_entity_slug_key;

ALTER TABLE wiki_pages
    DROP CONSTRAINT IF EXISTS wiki_pages_entity_pair,
    DROP CONSTRAINT IF EXISTS wiki_pages_one_parent,
    DROP COLUMN IF EXISTS entity_id,
    DROP COLUMN IF EXISTS entity_type,
    ALTER COLUMN space_id SET NOT NULL;

ALTER TABLE wiki_type_configs
    DROP COLUMN IF EXISTS entity_discriminator,
    DROP COLUMN IF EXISTS entity_kind;

DROP TABLE IF EXISTS wiki_entity_kinds;

COMMIT;
