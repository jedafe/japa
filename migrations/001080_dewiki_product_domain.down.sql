-- Restore the product-domain wiki bindings removed by the up migration —
-- kind registry rows plus the 001079 provisioning chain (space_types →
-- wiki_type_configs → categories → page templates). Deleted entity-bound
-- wiki_pages (and their cascaded revisions/feedback) are NOT recoverable.
BEGIN;

INSERT INTO wiki_entity_kinds (entity_kind, label, source_table, id_column, slug_column) VALUES
    ('product',  'Products',  'products',            'id', 'slug'),
    ('provider', 'Providers', 'affiliate_providers', 'id', 'slug'),
    ('deal',     'Deals',     'coupons',             'id', 'slug')
ON CONFLICT (entity_kind) DO NOTHING;

DO $$
DECLARE
    pt RECORD;
    stid uuid;
    gid uuid;
    wcid uuid;
BEGIN
    FOR pt IN SELECT slug, label, icon FROM product_types WHERE is_enabled LOOP
        INSERT INTO space_types (name, slug, description, icon, category, is_system, wiki_only)
        VALUES (pt.label, pt.slug, 'Compare ' || pt.label, pt.icon, NULL, true, true)
        ON CONFLICT (slug) DO NOTHING
        RETURNING id INTO stid;

        IF stid IS NULL THEN
            SELECT id INTO stid FROM space_types WHERE slug = pt.slug;
        END IF;

        INSERT INTO wiki_type_configs (space_type_id, enabled, label, icon, entity_kind, entity_discriminator, seed_stubs, position)
        VALUES (stid, true, pt.label, pt.icon, 'product',
                jsonb_build_object('type_slug', pt.slug), false, 0)
        ON CONFLICT (space_type_id) DO UPDATE
        SET entity_kind = 'product',
            entity_discriminator = jsonb_build_object('type_slug', pt.slug),
            enabled = true;

        SELECT id INTO gid FROM custom_field_groups
        WHERE slug = 'wiki_' || replace(pt.slug, '-', '_') || '_overview';

        INSERT INTO wiki_type_categories (space_type_id, slug, category, label, field_group_id, position, is_system)
        VALUES (stid, 'overview', 'overview', 'Overview', gid, 0, true)
        ON CONFLICT DO NOTHING
        RETURNING id INTO wcid;

        IF wcid IS NULL THEN
            SELECT id INTO wcid FROM wiki_type_categories
            WHERE space_type_id = stid AND slug = 'overview';
        END IF;

        INSERT INTO wiki_type_page_templates (category_id, slug, label, field_group_id, position, is_system, preview_limit)
        VALUES (wcid, 'overview', 'Overview', gid, 0, true, 5)
        ON CONFLICT DO NOTHING;
    END LOOP;
END $$;

COMMIT;
