-- 001117_guide_category_integrity.up.sql
-- §14: enforce guide_category integrity at the database level so every
-- writer path — content.Service, the intelligence promoter, the
-- scholarships admin insert, assessment stubs, and any future direct SQL —
-- gets the same guarantees the API layer validates:
--
--   1. contents.guide_category must reference a registered
--      custom_field_entity_types 'guide_category:{slug}' row. Without this,
--      a direct writer can persist an unknown slug and the category's
--      required-field gate passes vacuously at publish time.
--
--   2. A guide_category entity type cannot be deleted while live contents
--      still reference its slug — the service-level in-use count raced the
--      delete (TOCTOU); the trigger makes the check atomic.
--
-- The validate trigger skips unchanged values on UPDATE so rows carrying a
-- pre-migration stale slug can still be edited for other columns without
-- suddenly failing.

CREATE OR REPLACE FUNCTION contents_validate_guide_category()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    -- Only enforce when the column is actually changing — a legacy stale
    -- value untouched by the write must not block unrelated edits.
    IF TG_OP = 'UPDATE' AND NEW.guide_category IS NOT DISTINCT FROM OLD.guide_category THEN
        RETURN NEW;
    END IF;

    IF NEW.guide_category IS NULL OR btrim(NEW.guide_category) = '' THEN
        RETURN NEW;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM custom_field_entity_types
        WHERE value = 'guide_category:' || NEW.guide_category
    ) THEN
        RAISE EXCEPTION 'invalid guide_category %', NEW.guide_category
            USING ERRCODE = '23503';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_contents_guide_category
    BEFORE INSERT OR UPDATE OF guide_category ON contents
    FOR EACH ROW
    EXECUTE FUNCTION contents_validate_guide_category();

CREATE OR REPLACE FUNCTION entity_types_guard_guide_category_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    v_slug text;
BEGIN
    IF OLD.value NOT LIKE 'guide_category:%' THEN
        RETURN OLD;
    END IF;

    v_slug := substring(OLD.value FROM length('guide_category:') + 1);

    IF EXISTS (
        SELECT 1
        FROM contents
        WHERE contents.guide_category = v_slug
          AND contents.deleted_at IS NULL
    ) THEN
        RAISE EXCEPTION 'guide_category % still referenced by contents', v_slug
            USING ERRCODE = '23503';
    END IF;

    RETURN OLD;
END;
$$;

CREATE TRIGGER trg_entity_types_guide_category_delete
    BEFORE DELETE ON custom_field_entity_types
    FOR EACH ROW
    EXECUTE FUNCTION entity_types_guard_guide_category_delete();

-- Backfill og_image_url references: contents.og_image_url can carry a
-- /media/serve/{bucket}/{path} URL — tracked under field_key='og_image'
-- alongside the runtime sync added with this change. Same matching rules
-- as the 001116 body backfill (bucket/path regex on the serve URL).
INSERT INTO content_media_fields (content_id, media_id, field_key, position)
SELECT c.id, m.id, 'og_image', 0
FROM contents c
JOIN media m
  ON m.storage_bucket = substring(c.og_image_url FROM 'media/serve/([a-z0-9_-]+)/')
 AND m.storage_path = substring(c.og_image_url FROM 'media/serve/[a-z0-9_-]+/([0-9A-Za-z_\-\./%]+)')
WHERE c.og_image_url LIKE '%media/serve/%'
  AND c.deleted_at IS NULL
ON CONFLICT (content_id, field_key, position) DO NOTHING;
