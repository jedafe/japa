ALTER TABLE learning_tracks DROP CONSTRAINT IF EXISTS learning_tracks_slug_url_safe;
ALTER TABLE learning_modules DROP CONSTRAINT IF EXISTS learning_modules_slug_url_safe;
ALTER TABLE learning_lessons DROP CONSTRAINT IF EXISTS learning_lessons_slug_url_safe;
