-- Seed the org_type select custom field in the organization_fields group.
-- The /organizations/meta endpoint reads options from this field's options
-- JSONB, so admins can add/remove/relabel org types via the Custom Fields
-- admin UI without a dedicated table.
-- Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
--      japaconnect-engineering/sections/08-custom-field-architecture.md

-- The organization_fields system group is seeded in migration 000008 with
-- a fixed UUID (...045). Insert the org_type field on every existing tenant
-- that has the group.
INSERT INTO custom_fields (
    id, group_id, tenant_id, label, key, field_type, options,
    is_required, is_system, show_in_frontend, filterable, sortable, searchable, position
)
SELECT
    gen_random_uuid(),
    g.id,
    g.tenant_id,
    'Organization Type',
    'org_type',
    'select',
    '[
        {"value":"business","label":"Business"},
        {"value":"ngo","label":"NGO"},
        {"value":"government","label":"Government"},
        {"value":"embassy","label":"Embassy"},
        {"value":"university","label":"University"},
        {"value":"media","label":"Media"},
        {"value":"community","label":"Community"}
    ]'::jsonb,
    false,    -- is_required (org_type has a default on the organizations table)
    false,    -- is_system (editable via the custom fields admin UI)
    true,     -- show_in_frontend
    true,     -- filterable
    false,    -- sortable
    false,    -- searchable
    0         -- position
FROM custom_field_groups g
WHERE g.slug = 'organization_fields'
ON CONFLICT DO NOTHING;

-- Ensure the CHECK constraint on organizations.org_type is gone so new
-- types added via the custom field options editor are not rejected by the DB.
ALTER TABLE organizations DROP CONSTRAINT IF EXISTS organizations_org_type_check;
