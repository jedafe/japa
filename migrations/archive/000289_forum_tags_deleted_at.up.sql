-- Add deleted_at to forum_tags so SoftDeleteForumTag works.
-- The query SoftDeleteForumTag already references deleted_at but the column
-- was missing from the original CREATE TABLE (migration 000017), causing
-- DELETE /forum/tags/{id} to fail at runtime.
-- Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Tags)

ALTER TABLE forum_tags
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

-- Backfill existing rows so they are treated as not-deleted.
UPDATE forum_tags SET deleted_at = NULL WHERE deleted_at IS NULL;

-- Partial index mirroring the forum_threads pattern.
CREATE INDEX IF NOT EXISTS idx_forum_tags_tenant_active
    ON forum_tags(tenant_id) WHERE deleted_at IS NULL;
