-- 000787: two-way wiki -> comparison_item sync for cost_of_living.
-- Ref: sections/72-cost-of-living.md (Amendment: wiki city auto-materialization)
--      sections/80-destination-wiki.md
--
-- Extends 000786: items that exist ONLY because of wiki data leave the
-- calculator when that data does. An item is auto-removed (soft-deleted,
-- marked editorial_overrides._auto_removed='wiki_sync') when EITHER:
--   (a) its space has zero remaining non-empty wiki_city_cost_of_living cfv
--       rows after a cfv DELETE/UPDATE (evaluated at COMMIT via deferred
--       constraint trigger so delete-then-rewrite revisions don't fire it), or
--   (b) its city space is soft-deleted (the geo link / wiki page is gone).
-- Removal only applies to wiki-only items: an item with comparison_values
-- rows, an intel_record_id, or intel_cost_data is platform-curated and stays.
-- is_baseline items are never auto-removed. Auto-removed items RESURRECT on
-- fresh wiki data (marker proves it wasn't an admin delete); admin
-- soft-deletes still never resurrect.

-- =====================================================================
-- Shared removal core
-- =====================================================================

CREATE OR REPLACE FUNCTION prune_col_items_for_space(
    p_space_id uuid,
    p_require_no_wiki_data boolean
) RETURNS void AS $$
DECLARE
    v_type_id uuid;
BEGIN
    SELECT id INTO v_type_id
    FROM comparison_type_definitions
    WHERE slug = 'cost_of_living' AND is_system = true;
    IF NOT FOUND THEN
        RETURN;
    END IF;

    -- For cfv-driven removal, bail while any non-empty wiki CoL value remains.
    IF p_require_no_wiki_data AND EXISTS (
        SELECT 1
        FROM custom_field_values cfv
        JOIN custom_fields cf ON cf.id = cfv.field_id
        JOIN custom_field_groups cfg ON cfg.id = cf.group_id
        WHERE cfv.entity_type IN ('space', 'city')
          AND cfv.entity_id = p_space_id
          AND cfg.slug = 'wiki_city_cost_of_living'
          AND (cfv.value_text IS NOT NULL
               OR cfv.value_number IS NOT NULL
               OR cfv.value_bool IS NOT NULL
               OR cfv.value_date IS NOT NULL
               OR cfv.value_json IS NOT NULL)
    ) THEN
        RETURN;
    END IF;

    UPDATE comparison_items ci
    SET deleted_at = now(),
        editorial_overrides = ci.editorial_overrides || '{"_auto_removed":"wiki_sync"}'::jsonb
    WHERE ci.type_id = v_type_id
      AND ci.geo_space_id = p_space_id
      AND ci.deleted_at IS NULL
      AND ci.is_baseline = false
      AND ci.intel_record_id IS NULL
      AND COALESCE(ci.intel_cost_data, '{}'::jsonb) = '{}'::jsonb
      AND NOT EXISTS (
          SELECT 1 FROM comparison_values cv WHERE cv.item_id = ci.id
      );
END;
$$ LANGUAGE plpgsql;

-- =====================================================================
-- cfv DELETE/UPDATE -> deferred removal check (runs at COMMIT)
-- =====================================================================

CREATE OR REPLACE FUNCTION col_item_removed_wiki_cfv()
RETURNS trigger AS $$
BEGIN
    IF OLD.entity_type IN ('space', 'city') THEN
        -- Same advisory key as materialization so concurrent writers on the
        -- same space serialize against each other.
        PERFORM pg_advisory_xact_lock(
            hashtext('col_item_materialize'), hashtext(OLD.entity_id::text)
        );
        PERFORM prune_col_items_for_space(OLD.entity_id, true);
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_col_item_removed_wiki_cfv_del
    AFTER DELETE ON custom_field_values
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION col_item_removed_wiki_cfv();

-- UPDATE is checked against OLD.entity_id: a cfv row moved to another
-- entity or out of the CoL group can drain the old space's last value.
CREATE CONSTRAINT TRIGGER trg_col_item_removed_wiki_cfv_upd
    AFTER UPDATE ON custom_field_values
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION col_item_removed_wiki_cfv();

-- =====================================================================
-- City space soft-delete -> removal (wiki-only items; curated items stay
-- for admin repointing, per the dead-space pattern)
-- =====================================================================

CREATE OR REPLACE FUNCTION col_item_removed_space_deleted()
RETURNS trigger AS $$
BEGIN
    IF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL AND NEW.level = 'city' THEN
        PERFORM pg_advisory_xact_lock(
            hashtext('col_item_materialize'), hashtext(NEW.id::text)
        );
        PERFORM prune_col_items_for_space(NEW.id, false);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_col_item_removed_space_deleted
    AFTER UPDATE OF deleted_at ON spaces
    FOR EACH ROW
    EXECUTE FUNCTION col_item_removed_space_deleted();

-- =====================================================================
-- Materialization: resurrect auto-removed items on fresh wiki data
-- =====================================================================

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

    -- Live item: nothing to do.
    IF EXISTS (
        SELECT 1 FROM comparison_items
        WHERE type_id = v_type_id AND geo_space_id = NEW.entity_id
          AND deleted_at IS NULL
    ) THEN
        RETURN NEW;
    END IF;

    -- Soft-deleted + marked auto-removed: the wiki data is back, so is the
    -- city. Admin soft-deletes carry no marker and are never resurrected.
    UPDATE comparison_items
    SET deleted_at = NULL,
        last_updated_at = now(),
        editorial_overrides = editorial_overrides - '_auto_removed'
    WHERE type_id = v_type_id
      AND geo_space_id = NEW.entity_id
      AND deleted_at IS NOT NULL
      AND editorial_overrides->>'_auto_removed' = 'wiki_sync';
    IF FOUND THEN
        RETURN NEW;
    END IF;

    -- Any other tombstone = admin removal; skip.
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
