-- Register the new item kinds in wiki_entity_kinds so they index on
-- /wiki/e (§80 Product Domain). source_table/columns feed the directory's
-- count queries; kindDiscriminatorColumn (code) handles product's t.slug.
BEGIN;

INSERT INTO wiki_entity_kinds (entity_kind, label, source_table, id_column, slug_column) VALUES
    ('product',  'Products',  'products',             'id', 'slug'),
    ('provider', 'Providers', 'affiliate_providers',  'id', 'slug'),
    ('deal',     'Deals',     'coupons',              'id', 'slug')
ON CONFLICT (entity_kind) DO NOTHING;

COMMIT;
