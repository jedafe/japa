-- 001068_comparison_type_group_audit.up.sql
-- Comparison-type field-group audit (§30 Sub-spec 5): every type's
-- dimensions should generate from a group whose cfv bucket is writable by
-- a canonical surface — wiki field ops, guide-category fields, or intel
-- extraction/curation.
--
-- Fixes:
--   1. 'visa-types' was bound to organization_fields (org_type) — same seed
--      bug as courses/cost_of_living (001064). Repoint to
--      wiki_visa_route_overview, same group as 'visa-routes' and the per-kind
--      visa families.
--   2. 'banks' bound to organization_fields too — repoint to
--      wiki_bank_overview (the bank destination-wiki group).
--   3. Repair the existing 'Organization Type' dims: banks-type dims →
--      bank_type, visa-types-type dims → route_type (semantic equivalents).
--   4. Assign the unassigned canonical groups so ops can edit them:
--      geo_fields → country/state/city space types (wikifacts already writes
--      it via UUIDs; the assignment makes the facts wiki-editable), and the
--      country-scoped registration groups → space_type:country.

BEGIN;

UPDATE comparison_type_definitions t
SET field_group_id = g.id
FROM custom_field_groups g
WHERE g.slug = 'wiki_visa_route_overview' AND t.slug = 'visa-types';

UPDATE comparison_type_definitions t
SET field_group_id = g.id
FROM custom_field_groups g
WHERE g.slug = 'wiki_bank_overview' AND t.slug = 'banks';

UPDATE comparison_dimensions d
SET source_field_id = n.id
FROM custom_fields o
JOIN custom_field_groups og ON og.id = o.group_id AND og.slug = 'organization_fields'
JOIN custom_field_groups ng ON ng.slug = 'wiki_bank_overview'
JOIN custom_fields n ON n.group_id = ng.id AND n.key = 'bank_type'
WHERE d.source_field_id = o.id AND o.key = 'org_type'
  AND d.type_id = (SELECT id FROM comparison_type_definitions WHERE slug = 'banks');

UPDATE comparison_dimensions d
SET source_field_id = n.id
FROM custom_fields o
JOIN custom_field_groups og ON og.id = o.group_id AND og.slug = 'organization_fields'
JOIN custom_field_groups ng ON ng.slug = 'wiki_visa_route_overview'
JOIN custom_fields n ON n.group_id = ng.id AND n.key = 'route_type'
WHERE d.source_field_id = o.id AND o.key = 'org_type'
  AND d.type_id = (SELECT id FROM comparison_type_definitions WHERE slug = 'visa-types');

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, m.cls, NULL, 0
FROM custom_field_groups g
JOIN (VALUES
    ('geo_fields',                          'space_type:country'),
    ('geo_fields',                          'space_type:state'),
    ('geo_fields',                          'space_type:city'),
    ('bank_account_registration_fields',    'space_type:country'),
    ('business_registration_fields',        'space_type:country'),
    ('residents_permit_requirements_fields','space_type:country')
) AS m(grp, cls) ON m.grp = g.slug
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = g.id AND a.entity_class = m.cls AND a.entity_id IS NULL
);

COMMIT;
