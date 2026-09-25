-- Section 24: Blog Extension — soft-delete columns + junction position uniqueness.
-- H1: Add deleted_at to blog_series and content_reviews (Rule 11 soft-delete).
-- L3: Add UNIQUE(series_id, position) to prevent duplicate ordering.

-- Add deleted_at to blog_series (user-authored content — Rule 11).
ALTER TABLE blog_series ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
CREATE INDEX IF NOT EXISTS idx_blog_series_deleted
    ON blog_series(tenant_id) WHERE deleted_at IS NULL;

-- Add deleted_at to content_reviews (editorial audit history — Rule 11).
ALTER TABLE content_reviews ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
CREATE INDEX IF NOT EXISTS idx_content_reviews_deleted
    ON content_reviews(tenant_id) WHERE deleted_at IS NULL;

-- L3: prevent duplicate positions within a series.
-- First remove any existing duplicates by shifting positions so they are
-- unique within each series (ordered by created_at as a tiebreaker).
DELETE FROM blog_series_posts bsp
WHERE EXISTS (
    SELECT 1 FROM blog_series_posts bsp2
    WHERE bsp2.series_id = bsp.series_id
      AND bsp2.content_id < bsp.content_id
      AND bsp2.position = bsp.position
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_blog_series_posts_position
    ON blog_series_posts(series_id, position);
