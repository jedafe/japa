-- 000731_wiki_community_fields.up.sql
-- Wiki community fields (§80 ops revisions, §30 source_field_id).
--
-- Extends wiki_revisions so a revision can carry structured field/group
-- mutation ops (kind='fields'|'mixed') against a polymorphic target entity
-- (target_entity_type/target_entity_id) in addition to — or instead of —
-- TipTap page content. wiki_page_id becomes nullable for field-only
-- revisions that target a non-page entity.
--
-- comparison_dimensions.source_field_id links a comparison axis back to the
-- custom_fields row it was generated from (§30), and wiki_pages.
-- data_entity_type marks a wiki page as the structured-data page for an
-- entity class.
--
-- Also seeds three RBAC permission nodes (permissions + role_permissions,
-- same shape as 000730):
--   wiki.fields.edit    — community value editing (reputation-gated)
--   wiki.fields.propose — member-level field/group proposals
--   wiki.fields.manage  — field/group/assignment management + auto-approve
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
--      japaconnect-engineering/sections/30-comparison-system.md
--      japaconnect-engineering/sections/03-rbac.md

BEGIN;

-- 1. wiki_revisions: ops + polymorphic target columns; wiki_page_id nullable.
ALTER TABLE wiki_revisions
    ADD COLUMN kind text NOT NULL DEFAULT 'content',
    ADD COLUMN ops jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN target_entity_type text,
    ADD COLUMN target_entity_id uuid,
    ALTER COLUMN wiki_page_id DROP NOT NULL;

ALTER TABLE wiki_revisions
    ADD CONSTRAINT wiki_revisions_kind_check
        CHECK (kind IN ('content', 'fields', 'mixed'));

CREATE INDEX idx_wiki_revisions_target
    ON wiki_revisions (target_entity_type, target_entity_id);
CREATE INDEX idx_wiki_revisions_status_kind
    ON wiki_revisions (status, kind);

-- 2. comparison_dimensions: link a dimension to its source custom field.
ALTER TABLE comparison_dimensions
    ADD COLUMN source_field_id uuid REFERENCES custom_fields(id);

-- 3. wiki_pages: mark the entity class a data-backed wiki page describes.
ALTER TABLE wiki_pages
    ADD COLUMN data_entity_type text;

-- 4. Seed the three wiki.fields.* permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('wiki.fields.edit',    'Edit community field values on wiki entities (reputation-gated)'),
    ('wiki.fields.propose', 'Propose new fields/groups on wiki entities (member-level)'),
    ('wiki.fields.manage',  'Manage wiki fields, groups, and assignments; auto-approve field revisions (admin+)')
ON CONFLICT (key) DO NOTHING;

-- wiki.fields.edit + wiki.fields.propose — member+ roles (same grant set as
-- wiki.edit per 000491; reputation threshold enforced in the service layer).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('member', 'professional', 'agent', 'moderator', 'admin', 'super_admin')
  AND p.key IN ('wiki.fields.edit', 'wiki.fields.propose')
ON CONFLICT DO NOTHING;

-- wiki.fields.manage — admin + super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('admin', 'super_admin')
  AND p.key = 'wiki.fields.manage'
ON CONFLICT DO NOTHING;

COMMIT;
