-- 000787 down: drop two-way wiki sync, restore 000786 materialize-only body.
-- Auto-removed items stay soft-deleted (data, intentionally kept).

DROP TRIGGER IF EXISTS trg_col_item_removed_wiki_cfv_del ON custom_field_values;
DROP TRIGGER IF EXISTS trg_col_item_removed_wiki_cfv_upd ON custom_field_values;
DROP TRIGGER IF EXISTS trg_col_item_removed_space_deleted ON spaces;
DROP FUNCTION IF EXISTS col_item_removed_wiki_cfv();
DROP FUNCTION IF EXISTS col_item_removed_space_deleted();
DROP FUNCTION IF EXISTS prune_col_items_for_space(uuid, boolean);

CREATE OR REPLACE FUNCTION materialize_col_item_for_wiki_space()
RETURNS trigger AS $$
DECLARE
    v_space   record;
    v_type_id uuid;
    v_content_id uuid;
BEGIN
    IF NEW.entity_type NOT IN ('space', 'city') THEN
        RETURN NEW;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM custom_fields cf
        JOIN custom_field_groups cfg ON cfg.id = cf.group_id
        WHERE cf.id = NEW.field_id
          AND cfg.slug = 'wiki_city_cost_of_living'
    ) THEN
        RETURN NEW;
    END IF;

    SELECT id, name INTO v_space
    FROM spaces
    WHERE id = NEW.entity_id
      AND level = 'city'
      AND deleted_at IS NULL;
    IF NOT FOUND THEN
        RETURN NEW;
    END IF;

    SELECT id INTO v_type_id
    FROM comparison_type_definitions
    WHERE slug = 'cost_of_living' AND is_system = true;
    IF NOT FOUND THEN
        RETURN NEW;
    END IF;

    PERFORM pg_advisory_xact_lock(
        hashtext('col_item_materialize'), hashtext(NEW.entity_id::text)
    );

    IF EXISTS (
        SELECT 1 FROM comparison_items
        WHERE type_id = v_type_id AND geo_space_id = NEW.entity_id
    ) THEN
        RETURN NEW;
    END IF;

    SELECT content_id INTO v_content_id
    FROM comparison_items
    WHERE type_id = v_type_id
      AND content_id IS NOT NULL
      AND deleted_at IS NULL
    LIMIT 1;

    INSERT INTO comparison_items (title, content_id, geo_space_id, type_id, position)
    SELECT v_space.name, v_content_id, NEW.entity_id, v_type_id,
           COALESCE(MAX(position), -1) + 1
    FROM comparison_items
    WHERE type_id = v_type_id AND deleted_at IS NULL;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
