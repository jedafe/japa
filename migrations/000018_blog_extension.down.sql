-- Down migration for 000018_blog_extension.sql
-- Drops blog series posts, content reviews, content featured, and blog series tables.
DROP TABLE IF EXISTS blog_series_posts;
DROP TABLE IF EXISTS content_reviews;
DROP TABLE IF EXISTS content_featured;
DROP TABLE IF EXISTS blog_series;
