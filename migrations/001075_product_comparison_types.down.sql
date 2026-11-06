BEGIN;

DELETE FROM comparison_type_field_groups
WHERE type_id IN (SELECT id FROM comparison_type_definitions
    WHERE slug IN (SELECT slug FROM product_types));

DELETE FROM comparison_type_definitions
WHERE slug IN (SELECT slug FROM product_types);

DELETE FROM custom_field_groups
WHERE slug IN (SELECT 'wiki_' || replace(slug, '-', '_') || '_overview' FROM product_types);

COMMIT;
