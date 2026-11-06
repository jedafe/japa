BEGIN;
-- Config was written once by 001075; restoring the old shape is
-- lossy-but-harmless (auto_fields was already dead).
UPDATE comparison_type_definitions t
SET config = jsonb_build_object('subtype', t.config->>'subtype')
WHERE t.slug IN (SELECT slug FROM product_types);
COMMIT;
