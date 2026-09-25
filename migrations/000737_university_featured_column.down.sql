-- Revert 000737_university_featured_column.up.sql.
ALTER TABLE public.learning_tracks
    DROP COLUMN IF EXISTS university_featured;
