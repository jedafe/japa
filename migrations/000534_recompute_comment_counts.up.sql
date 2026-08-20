-- Backfill denormalized comment counts so they reflect the total number of
-- published, non-deleted comments (including replies). This also updates the
-- corresponding content_scores row used by feed ranking, without inserting
-- new score rows (which the engagement worker / batch scoring creates as needed).
--
-- Ref: Section 49 (Comments & Replies), BUG-011 (counter drift).

BEGIN;

-- contents.comment_count is displayed on feed/blog cards and content singles.
UPDATE contents c
SET comment_count = (
    SELECT COUNT(*)
    FROM comments cm
    WHERE cm.content_id = c.id
      AND cm.tenant_id = c.tenant_id
      AND cm.deleted_at IS NULL
      AND cm.status = 'published'
)
WHERE c.deleted_at IS NULL;

-- comments.reply_count is used when rendering "X replies" on each comment.
UPDATE comments p
SET reply_count = (
    SELECT COUNT(*)
    FROM comments r
    WHERE r.parent_id = p.id
      AND r.tenant_id = p.tenant_id
      AND r.deleted_at IS NULL
      AND r.status = 'published'
)
WHERE p.deleted_at IS NULL;

-- content_scores.comment_count feeds velocity/ranking calculations.
UPDATE content_scores cs
SET comment_count = (
    SELECT COUNT(*)
    FROM comments cm
    WHERE cm.content_id = cs.content_id
      AND cm.deleted_at IS NULL
      AND cm.status = 'published'
);

COMMIT;
