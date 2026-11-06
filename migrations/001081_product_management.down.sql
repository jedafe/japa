-- 001081_product_management.down.sql
BEGIN;

DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
      'products.list','products.view','products.create','products.update','products.delete',
      'product_types.list','product_types.view','product_types.create',
      'product_types.update','product_types.delete'));
DELETE FROM permissions WHERE key IN (
      'products.list','products.view','products.create','products.update','products.delete',
      'product_types.list','product_types.view','product_types.create',
      'product_types.update','product_types.delete');

DELETE FROM custom_field_entity_types WHERE value = 'product';

DELETE FROM custom_field_group_assignments a
USING product_types pt, custom_field_groups g
WHERE a.group_id = g.id AND a.entity_class = pt.slug AND a.entity_id IS NULL
  AND g.slug LIKE 'wiki\_' || replace(pt.slug, '-', '_') || '\_%' ESCAPE '\';

DROP TABLE product_deals;

COMMIT;
