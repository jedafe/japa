-- Restore learning_tracks.university_featured: the column was added by
-- archived migration 000218 (ADR-063 §5) but was lost when the migration
-- chain was squashed into the 000735 baseline. The column is live code —
-- SetUniversityTrackFeatured + the admin feature/unfeature handler
-- (university.course.feature) reference it, and its absence broke
-- `sqlc generate` for every query in the repo.
ALTER TABLE public.learning_tracks
    ADD COLUMN IF NOT EXISTS university_featured boolean NOT NULL DEFAULT false;
