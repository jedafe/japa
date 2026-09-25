-- 000620_story_meta_unique_content.down.sql
-- Drops the uniqueness constraint (duplicate rows are not restored).
ALTER TABLE story_meta
    DROP CONSTRAINT IF EXISTS story_meta_content_tenant_key;
