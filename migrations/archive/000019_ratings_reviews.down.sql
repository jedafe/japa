-- Down migration for 000019_ratings_reviews.sql
-- Drops review aggregates and reviews tables.
DROP TABLE IF EXISTS review_aggregates;
DROP TABLE IF EXISTS reviews;
