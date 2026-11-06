-- 001068_comparison_type_group_audit.down.sql
BEGIN;

UPDATE comparison_type_definitions t
SET field_group_id = g.id
FROM custom_field_groups g
WHERE g.slug = 'organization_fields' AND t.slug IN ('visa-types', 'banks');

UPDATE comparison_dimensions d
SET source_field_id = o.id
FROM custom_fields n
JOIN custom_field_groups ng ON ng.id = n.group_id AND ng.slug IN ('wiki_bank_overview','wiki_visa_route_overview')
JOIN custom_fields o ON o.key = 'org_type'
JOIN custom_field_groups og ON og.id = o.group_id AND og.slug = 'organization_fields'
WHERE d.source_field_id = n.id AND n.key IN ('bank_type','route_type');

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id AND a.entity_id IS NULL AND (
    (g.slug = 'geo_fields' AND a.entity_class IN ('space_type:country','space_type:state','space_type:city'))
    OR (g.slug IN ('bank_account_registration_fields','business_registration_fields','residents_permit_requirements_fields')
        AND a.entity_class = 'space_type:country')
);

COMMIT;
