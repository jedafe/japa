-- De-wiki the product domain (§80 amendment): products, providers and deals
-- are comparison/marketplace entities, not community wiki entries — like
-- mentors they stay comparable via their JCE resolvers but no longer
-- surface under /wiki/e (index, kind listings, entity pages) or anywhere
-- in the wiki type machinery.
--
-- Order matters:
--   1. wiki_pages.entity_type FKs into wiki_entity_kinds — the entity pages
--      must go first. Revisions + feedback cascade with the page rows.
--   2. The product-type wiki chains (001079 provisioning: space_types →
--      wiki_type_configs → wiki_type_categories → page templates) are torn
--      down — the config rows also FK into wiki_entity_kinds, and the
--      space_types would linger as dead wiki_only types in /wiki/types.
--   3. The kind registry rows then delete cleanly.
--
-- comparison_type_definitions, pools, products, product_providers and cfv
-- are untouched — /compare keeps working.

BEGIN;

DELETE FROM wiki_pages
WHERE entity_type IN ('product', 'provider', 'deal');

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

DELETE FROM space_types
WHERE slug IN (SELECT slug FROM product_types);

DELETE FROM wiki_entity_kinds
WHERE entity_kind IN ('product', 'provider', 'deal');

COMMIT;
