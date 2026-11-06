BEGIN;

DELETE FROM wiki_type_page_templates
WHERE category_id IN (
    SELECT wtc.id FROM wiki_type_categories wtc
    JOIN space_types st ON st.id = wtc.space_type_id
    WHERE st.slug IN (SELECT slug FROM product_types));

DELETE FROM wiki_type_categories
WHERE space_type_id IN (SELECT id FROM space_types
    WHERE slug IN (SELECT slug FROM product_types));

DELETE FROM wiki_type_configs
WHERE space_type_id IN (SELECT id FROM space_types
    WHERE slug IN (SELECT slug FROM product_types));

DELETE FROM space_types WHERE slug IN (SELECT slug FROM product_types);

COMMIT;
