-- Down migration for 000185.

DROP INDEX IF EXISTS idx_blog_series_posts_position;
DROP INDEX IF EXISTS idx_content_reviews_deleted;
DROP INDEX IF EXISTS idx_blog_series_deleted;
ALTER TABLE content_reviews DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE blog_series DROP COLUMN IF EXISTS deleted_at;
