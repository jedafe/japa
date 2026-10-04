-- 000786: auto-materialize cost_of_living comparison_items from wiki cfv.
-- Ref: sections/72-cost-of-living.md (Amendment: wiki city auto-materialization)
--      sections/80-destination-wiki.md
--
-- The §72 calculator catalogue is derived from comparison_items, while the
-- §80 wiki stores community cost data in custom_field_values on city spaces.
-- Previously a wiki city with cost data but no comparison_item was invisible
-- to the calculator. This trigger closes the gap: any cfv write whose field
-- belongs to the wiki_city_cost_of_living group on a live city-level space
-- creates the item if none exists (live OR soft-deleted — an admin delete is
-- never resurrected by later wiki edits).

CREATE OR REPLACE FUNCTION materialize_col_item_for_wiki_space()
RETURNS trigger AS $$
DECLARE
    v_space   record;
    v_type_id uuid;
    v_content_id uuid;
BEGIN
    -- Cheap pre-filter: space-entity writes only.
    IF NEW.entity_type NOT IN ('space', 'city') THEN
        RETURN NEW;
    END IF;

    -- Field must belong to the wiki city cost-of-living group.
    IF NOT EXISTS (
        SELECT 1
        FROM custom_fields cf
        JOIN custom_field_groups cfg ON cfg.id = cf.group_id
        WHERE cf.id = NEW.field_id
          AND cfg.slug = 'wiki_city_cost_of_living'
    ) THEN
        RETURN NEW;
    END IF;

    -- Entity must be a live city-level space.
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

    -- Serialize concurrent materialization for the same space.
    PERFORM pg_advisory_xact_lock(
        hashtext('col_item_materialize'), hashtext(NEW.entity_id::text)
    );

    -- Never duplicate, never resurrect an admin soft-delete.
    IF EXISTS (
        SELECT 1 FROM comparison_items
        WHERE type_id = v_type_id AND geo_space_id = NEW.entity_id
    ) THEN
        RETURN NEW;
    END IF;

    -- Attach to the shared CoL contents bucket when derivable; the column is
    -- nullable so the item still lists without it.
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

CREATE TRIGGER trg_col_item_from_wiki_cfv
    AFTER INSERT OR UPDATE ON custom_field_values
    FOR EACH ROW
    EXECUTE FUNCTION materialize_col_item_for_wiki_space();

-- Backfill: materialize items for every city space that already has
-- wiki_city_cost_of_living data but no comparison_item yet. Idempotent via
-- the NOT EXISTS guard.
INSERT INTO comparison_items (title, content_id, geo_space_id, type_id, position)
SELECT s.name,
       (SELECT content_id FROM comparison_items ci2
         WHERE ci2.type_id = t.id AND ci2.content_id IS NOT NULL
           AND ci2.deleted_at IS NULL LIMIT 1),
       s.id,
       t.id,
       (SELECT COALESCE(MAX(ci3.position), -1) FROM comparison_items ci3
         WHERE ci3.type_id = t.id AND ci3.deleted_at IS NULL)
         + ROW_NUMBER() OVER (ORDER BY s.name)
FROM spaces s
CROSS JOIN comparison_type_definitions t
WHERE t.slug = 'cost_of_living' AND t.is_system = true
  AND s.level = 'city' AND s.deleted_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM custom_field_values cfv
      JOIN custom_fields cf ON cf.id = cfv.field_id
      JOIN custom_field_groups cfg ON cfg.id = cf.group_id
      WHERE cfv.entity_type IN ('space', 'city')
        AND cfv.entity_id = s.id
        AND cfg.slug = 'wiki_city_cost_of_living'
  )
  AND NOT EXISTS (
      SELECT 1 FROM comparison_items ci4
      WHERE ci4.type_id = t.id AND ci4.geo_space_id = s.id
  );
