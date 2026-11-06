-- Backfill the §80 provisioning chain for the 8 seeded product_types:
-- each gets a wiki_{type}_overview field group, a subtype-filtered
-- comparison_type_definitions row, and a default pool member — matching
-- what POST /admin/wiki/product-types does for new types.
BEGIN;

DO $$
DECLARE
    pt RECORD;
    gid uuid;
    tid uuid;
BEGIN
    FOR pt IN SELECT slug, label, icon FROM product_types WHERE is_enabled LOOP
        INSERT INTO custom_field_groups (name, slug, is_system)
        VALUES (pt.label || ' — Overview', 'wiki_' || replace(pt.slug, '-', '_') || '_overview', true)
        ON CONFLICT (slug) DO NOTHING
        RETURNING id INTO gid;

        IF gid IS NULL THEN
            SELECT id INTO gid FROM custom_field_groups
            WHERE slug = 'wiki_' || replace(pt.slug, '-', '_') || '_overview';
        END IF;

        INSERT INTO comparison_type_definitions
            (name, slug, description, icon, field_group_id, item_label, is_system, config)
        VALUES (
            pt.label,
            pt.slug,
            'Compare ' || pt.label,
            pt.icon,
            gid,
            pt.label,
            false,
            jsonb_build_object('subtype', pt.slug,
                'auto_fields', jsonb_build_array('rating_avg','review_count','has_deal','price_from'))
        )
        ON CONFLICT (slug) DO NOTHING
        RETURNING id INTO tid;

        IF tid IS NULL THEN
            SELECT id INTO tid FROM comparison_type_definitions WHERE slug = pt.slug;
        END IF;

        INSERT INTO comparison_type_field_groups (type_id, group_id, category, label, position, is_default)
        VALUES (tid, gid, 'overview', 'Overview', 0, true)
        ON CONFLICT DO NOTHING;
    END LOOP;
END $$;

COMMIT;
