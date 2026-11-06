-- Review fix: seeded product comparison types carry dead 'auto_fields'
-- and are missing 'item_ref_types' — the key that routes item search to
-- the 'product' resolver. Rewrite config to the canonical shape.
BEGIN;

UPDATE comparison_type_definitions t
SET config = jsonb_build_object(
        'subtype', t.config->>'subtype',
        'item_ref_types', '["product","comparison"]'::jsonb,
        'max_item_count', 5,
        'default_item_count', 2
    )
WHERE t.slug IN (SELECT slug FROM product_types)
  AND t.config->>'item_ref_types' IS NULL;

COMMIT;
