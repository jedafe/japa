-- 000650_forum_thread_delete_sync_content.up.sql
-- When a forum_threads row is hard-deleted (e.g. via FK CASCADE from users or
-- a manual/test cleanup), also soft-delete the backing contents row so the
-- post does not continue to appear in discovery feeds.
-- Ref: forum_threads.content_id -> contents(id)

BEGIN;

CREATE OR REPLACE FUNCTION trg_soft_delete_forum_thread_content()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.content_id IS NOT NULL THEN
        UPDATE contents
        SET deleted_at = now(),
            updated_at = now(),
            status = 'archived'
        WHERE id = OLD.content_id
          AND deleted_at IS NULL;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS soft_delete_forum_thread_content ON forum_threads;

CREATE TRIGGER soft_delete_forum_thread_content
    BEFORE DELETE ON forum_threads
    FOR EACH ROW
    EXECUTE FUNCTION trg_soft_delete_forum_thread_content();

-- One-time cleanup of any existing discussion contents whose forum thread is
-- gone or soft-deleted. Without this they would keep surfacing in feeds.
UPDATE contents
SET deleted_at = now(),
    updated_at = now(),
    status = 'archived'
WHERE content_type = 'discussion'
  AND deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM forum_threads ft
      WHERE ft.content_id = contents.id
        AND ft.deleted_at IS NULL
  );

COMMIT;
