BEGIN;

DELETE FROM content_type_jsonld_templates WHERE content_type = 'review';
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('review.create', 'agent.content_type.review'));
DELETE FROM permissions WHERE key IN ('review.create', 'agent.content_type.review');

DELETE FROM product_types;

DROP TABLE product_providers;
DROP TABLE products;
DROP TABLE product_types;

COMMIT;
