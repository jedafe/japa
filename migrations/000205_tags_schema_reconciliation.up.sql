-- Tags schema reconciliation (Section 08).
-- Adds spec-required columns missing from migration 000012:
--   rules, cover_media_id, content_count, is_system, field_values.
-- Recreates idx_tags_tenant as (tenant_id, follower_count DESC) per spec.
-- Adds a trigger to maintain tags.content_count on content_tags insert/delete.

-- =====================
-- 1. Add missing columns to tags
-- =====================
ALTER TABLE tags ADD COLUMN IF NOT EXISTS rules          text;
ALTER TABLE tags ADD COLUMN IF NOT EXISTS cover_media_id uuid REFERENCES media(id) ON DELETE SET NULL;
ALTER TABLE tags ADD COLUMN IF NOT EXISTS content_count  int  NOT NULL DEFAULT 0;
ALTER TABLE tags ADD COLUMN IF NOT EXISTS is_system      boolean NOT NULL DEFAULT false;
ALTER TABLE tags ADD COLUMN IF NOT EXISTS field_values   jsonb NOT NULL DEFAULT '{}';

-- =====================
-- 2. Recreate idx_tags_tenant per spec (tenant_id, follower_count DESC)
-- =====================
DROP INDEX IF EXISTS idx_tags_tenant;
CREATE INDEX idx_tags_tenant ON tags (tenant_id, follower_count DESC);

-- =====================
-- 3. Backfill content_count from existing content_tags
-- =====================
UPDATE tags t
SET content_count = COALESCE(
    (SELECT COUNT(*) FROM content_tags ct WHERE ct.tag_id = t.id),
    0
);

-- =====================
-- 4. Trigger to maintain content_count on content_tags changes
-- =====================
CREATE OR REPLACE FUNCTION fn_tags_content_count_sync()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE tags SET content_count = content_count + 1 WHERE id = NEW.tag_id;
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE tags SET content_count = content_count - 1 WHERE id = OLD.tag_id;
        RETURN OLD;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_tags_content_count_insert ON content_tags;
CREATE TRIGGER trg_tags_content_count_insert
    AFTER INSERT ON content_tags
    FOR EACH ROW EXECUTE FUNCTION fn_tags_content_count_sync();

DROP TRIGGER IF EXISTS trg_tags_content_count_delete ON content_tags;
CREATE TRIGGER trg_tags_content_count_delete
    AFTER DELETE ON content_tags
    FOR EACH ROW EXECUTE FUNCTION fn_tags_content_count_sync();
