-- 001045_bank_entity_split.up.sql
-- Banks are informational wiki entities, not platform organizations. The
-- 001036 seed stored them in `organizations` (verified=true, admin-owned),
-- which surfaced ~300 real banks as "verified" org profiles the actual
-- institutions never claimed — a fabricated-endorsement signal. Move them
-- to a dedicated `banks` table, mirroring the `embassies` pattern: wiki
-- entity_kind 'bank' reads it, and `organizations` only holds orgs with a
-- real operator. UUIDs are preserved so wiki_pages.entity_id and
-- custom_field_values.entity_id stay valid — only entity_type retags.
BEGIN;

CREATE TABLE banks (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         text NOT NULL UNIQUE,
    name         text NOT NULL,
    description  text,
    website      text,
    geo_space_id uuid REFERENCES spaces(id) ON DELETE RESTRICT,
    country_code text,
    field_values jsonb NOT NULL DEFAULT '{}'::jsonb,
    is_active    boolean NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX banks_geo_space ON banks (geo_space_id) WHERE is_active;

-- Move the seeded bank directory rows. The 'bank-{country}-{name}' slug
-- namespace is the 001036 seed convention; a user-created org with
-- org_type='bank' outside that pattern stays an organization.
INSERT INTO banks (id, slug, name, description, website, geo_space_id, country_code, field_values, is_active, created_at, updated_at)
SELECT id, slug, name, description, website_url, geo_space_id, country_code, field_values,
       (status = 'active'), created_at, updated_at
FROM organizations
WHERE org_type = 'bank' AND slug LIKE 'bank-%' AND deleted_at IS NULL;

-- Retag bound wiki data from the generic 'organization' kind to 'bank'.
-- cfv has a (field_id, entity_type, entity_id) unique key — drop any
-- 'organization' twin of an existing 'bank' row first so the retag cannot
-- collide.
DELETE FROM custom_field_values a
USING custom_field_values b
WHERE a.entity_type = 'organization'
  AND b.entity_type = 'bank'
  AND a.field_id = b.field_id
  AND a.entity_id = b.entity_id
  AND a.entity_id IN (SELECT id FROM banks);

UPDATE custom_field_values SET entity_type = 'bank'
WHERE entity_type = 'organization' AND entity_id IN (SELECT id FROM banks);

UPDATE wiki_pages SET entity_type = 'bank'
WHERE entity_type = 'organization' AND entity_id IN (SELECT id FROM banks);

UPDATE wiki_revisions SET target_entity_type = 'bank'
WHERE target_entity_type = 'organization' AND target_entity_id IN (SELECT id FROM banks);

-- Registry + type binding: the 'bank' space_type now binds entity_kind
-- 'bank' (undiscriminated — every banks row is a bank). The 'organization'
-- kind stays registered for future real-org wiki bindings.
INSERT INTO wiki_entity_kinds (entity_kind, source_table, id_column, slug_column, label)
VALUES ('bank', 'banks', 'id', 'slug', 'Bank')
ON CONFLICT (entity_kind) DO NOTHING;

UPDATE wiki_type_configs
SET entity_kind = 'bank', entity_discriminator = NULL
WHERE space_type_id = '00000000-0000-0000-0000-00000000007c';

-- cfv registry: 'bank' becomes a first-class value entity type.
INSERT INTO custom_field_entity_types (value, label, is_system, sort_order)
VALUES ('bank', 'Bank', true, 8)
ON CONFLICT (value) DO NOTHING;

-- Sever the moved rows' org-scoped dependents (FK NO ACTION/RESTRICT plus
-- polymorphic follows), then remove them from organizations. CASCADE and
-- SET NULL FKs (organization_members, invitations, boosts, coupons,
-- ad_campaigns, contents, service_listing_meta, community_organisations)
-- handle themselves on delete.
DELETE FROM ownership_claims WHERE organization_id IN (SELECT id FROM banks);
DELETE FROM verification_requests WHERE organization_id IN (SELECT id FROM banks);
DELETE FROM tips WHERE recipient_organization_id IN (SELECT id FROM banks);
DELETE FROM service_bookings WHERE org_id IN (SELECT id FROM banks);
UPDATE affiliate_providers SET organization_id = NULL WHERE organization_id IN (SELECT id FROM banks);
DELETE FROM dm_messages WHERE conversation_id IN (
    SELECT id FROM dm_conversations WHERE org_id IN (SELECT id FROM banks));
DELETE FROM dm_conversations WHERE org_id IN (SELECT id FROM banks);
DELETE FROM follows WHERE entity_type = 'organization' AND entity_id IN (SELECT id FROM banks);

DELETE FROM organizations WHERE id IN (SELECT id FROM banks);

COMMIT;
