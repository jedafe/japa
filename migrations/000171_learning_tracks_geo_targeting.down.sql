-- Reverse: drop the geo_targeting column and its partial index.
DROP INDEX IF EXISTS idx_learning_tracks_geo_space;
ALTER TABLE learning_tracks DROP COLUMN IF EXISTS geo_space_id;
