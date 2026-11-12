BEGIN;

DELETE FROM comparison_type_field_groups
WHERE type_id IN (
    SELECT id FROM comparison_type_definitions
    WHERE slug IN ('regions', 'programs', 'attractions')
);

DELETE FROM comparison_type_definitions
WHERE slug IN ('regions', 'programs', 'attractions');

COMMIT;
