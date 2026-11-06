-- Review → product rating aggregation (§80 Product Domain): the review
-- content type carries {product_slug, rating, providers[]} in
-- contents.field_values. Rather than hooking every publish path (create,
-- moderation approve, admin publish), a trigger keeps
-- products.rating_avg/review_count in sync over published reviews only.
BEGIN;

CREATE OR REPLACE FUNCTION sync_product_review_stats() RETURNS trigger AS $$
DECLARE
    tgt_slug text;
BEGIN
    -- content_type is the same column on insert/update/delete; only the
    -- WHEN-in-trigger approach broke (DELETE can't see NEW), so guard here.
    IF COALESCE(NEW.content_type, OLD.content_type) <> 'review' THEN
        RETURN COALESCE(NEW, OLD);
    END IF;
    tgt_slug := COALESCE(
        NEW.field_values->>'product_slug',
        OLD.field_values->>'product_slug'
    );
    IF tgt_slug IS NULL THEN RETURN COALESCE(NEW, OLD); END IF;

    UPDATE products SET
        review_count = (
            SELECT count(*) FROM contents
            WHERE content_type = 'review' AND status = 'published'
              AND field_values->>'product_slug' = tgt_slug
        ),
        rating_avg = (
            SELECT round(avg((field_values->>'rating')::numeric), 2)
            FROM contents
            WHERE content_type = 'review' AND status = 'published'
              AND field_values->>'product_slug' = tgt_slug
              AND (field_values->>'rating') ~ '^\d+(\.\d+)?$'
        )
    WHERE slug = tgt_slug;

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_sync_product_review_stats ON contents;
CREATE TRIGGER trg_sync_product_review_stats
    AFTER INSERT OR UPDATE OF status, field_values OR DELETE ON contents
    FOR EACH ROW
    EXECUTE FUNCTION sync_product_review_stats();

COMMIT;
