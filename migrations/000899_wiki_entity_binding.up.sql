-- 000899_wiki_entity_binding.up.sql
-- JCE ↔ Wiki field-group mapping, phase M1 (schema):
-- spec docs/superpowers/specs/2026-10-20-jce-wiki-field-group-mapping-design.md
--
-- Wiki types today bind to space_types only, and every wiki page binds a
-- space. Entity kinds that already power JCE comparisons (visa routes,
-- organizations/banks, course offerings, scholarships, embassies) cannot
-- have wiki pages. This migration adds the polymorphic binding:
--
--   * wiki_entity_kinds    — registry of entity kinds wiki pages may bind
--   * wiki_type_configs    — +entity_kind/entity_discriminator: a wiki type
--                            is space-bound (entity_kind NULL, current
--                            behavior) or entity-bound (pages of the type
--                            bind rows of entity_kind's source_table)
--   * wiki_pages           — +entity_type/entity_id; exactly one of
--                            space_id / entity_id set (precedent:
--                            wiki_revisions.target_entity_*)
--
-- The space_types row stays the type key for every wiki type — entity-bound
-- types get wiki_only space_types rows purely as registry keys (extends the
-- attraction pattern). No spaces are created for them.
--
-- entity_discriminator scopes a type to a slice of the source table
-- ({"visa_category":"student"} makes 'student visa' a wiki type over
-- visa_routes) — per-kind types share one table, one cfv entity_class.

BEGIN;

-- ---------------------------------------------------------------------------
-- Registry of entity kinds a wiki type / wiki page may bind.
-- ---------------------------------------------------------------------------
CREATE TABLE wiki_entity_kinds (
    entity_kind  text PRIMARY KEY,
    source_table text NOT NULL,
    id_column    text NOT NULL DEFAULT 'id',
    slug_column  text,                    -- NULL: kind has no slug column yet
    label        text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- A wiki type is space-bound (entity_kind NULL — unchanged behavior) or
-- entity-bound. The discriminator is a {column: value} equality map the
-- service applies when resolving which source-table rows belong to the type.
-- ---------------------------------------------------------------------------
ALTER TABLE wiki_type_configs
    ADD COLUMN entity_kind          text REFERENCES wiki_entity_kinds(entity_kind),
    ADD COLUMN entity_discriminator jsonb;

COMMENT ON COLUMN wiki_type_configs.entity_kind IS
    'NULL = space-bound wiki type. Set = pages bind entity rows of this kind';
COMMENT ON COLUMN wiki_type_configs.entity_discriminator IS
    '{column: value} equality filter selecting the source-table slice for this type (e.g. {"visa_category":"student"})';

-- ---------------------------------------------------------------------------
-- Pages may bind a space OR an entity row — exactly one.
-- ---------------------------------------------------------------------------
ALTER TABLE wiki_pages
    ALTER COLUMN space_id DROP NOT NULL,
    ADD COLUMN entity_type text REFERENCES wiki_entity_kinds(entity_kind),
    ADD COLUMN entity_id   uuid,
    ADD CONSTRAINT wiki_pages_one_parent
        CHECK (num_nonnulls(space_id, entity_id) = 1),
    ADD CONSTRAINT wiki_pages_entity_pair
        CHECK ((entity_type IS NULL) = (entity_id IS NULL));

-- (space_id, slug) stays unique for space-bound pages; entity pages get the
-- parallel unique key. Partial index so NULL entity_id rows are excluded.
CREATE UNIQUE INDEX wiki_pages_entity_slug_key
    ON wiki_pages (entity_type, entity_id, slug)
    WHERE entity_id IS NOT NULL;

CREATE INDEX idx_wiki_pages_entity
    ON wiki_pages (entity_type, entity_id)
    WHERE entity_id IS NOT NULL;

COMMIT;
