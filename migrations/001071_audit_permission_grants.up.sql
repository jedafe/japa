-- 001071_audit_permission_grants.up.sql
-- The 001068/001070 audit migrations assigned groups to entity classes but
-- skipped custom_field_permissions — ops submissions fail closed on missing
-- can_edit, so these fields were assigned but not editable. Grant the
-- standard convention: view for all user groups, edit for non-guest.

BEGIN;

INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, ug.id, true, ug.slug <> 'guest'
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id
CROSS JOIN user_groups ug
WHERE g.slug IN (
    'geo_fields',
    'bank_account_registration_fields',
    'business_registration_fields',
    'residents_permit_requirements_fields',
    'wiki_remittance_overview',
    'wiki_languagetest_overview',
    'wiki_travel_overview'
)
ON CONFLICT (field_id, user_group_id) DO NOTHING;

COMMIT;
