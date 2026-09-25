-- 000509_comment_views_table.up.sql
-- BUG-007: Creates the comment_views table and comments.view_count column
-- defined by Section 49 (Optional reply view tracking — admin-toggleable
-- per Section 08). Keyed by (comment_id, user_id); a DB trigger increments
-- comments.view_count on each insert.
-- Ref: japaconnect-engineering/sections/49-comments-replies.md

BEGIN;

-- =====================
-- comments.view_count: denormalized view counter
-- =====================
ALTER TABLE comments ADD COLUMN IF NOT EXISTS view_count int NOT NULL DEFAULT 0;

-- =====================
-- comment_views: per-user view tracking (Section 49)
-- =====================
CREATE TABLE IF NOT EXISTS comment_views (
    comment_id  uuid NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    viewed_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (comment_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_comment_views_comment ON comment_views (comment_id);
CREATE INDEX IF NOT EXISTS idx_comment_views_user ON comment_views (user_id);

-- =====================
-- Trigger: increment comments.view_count on comment_views insert
-- =====================
-- Uses INSERT ... ON CONFLICT DO NOTHING semantics so repeat views from the
-- same user don't double-count (the PK on (comment_id, user_id) deduplicates).
CREATE OR REPLACE FUNCTION trg_comment_views_increment()
RETURNS trigger AS $$
BEGIN
    UPDATE comments SET view_count = view_count + 1 WHERE id = NEW.comment_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_comment_views_after_insert ON comment_views;
CREATE TRIGGER trg_comment_views_after_insert
    AFTER INSERT ON comment_views
    FOR EACH ROW
    EXECUTE FUNCTION trg_comment_views_increment();

COMMIT;
