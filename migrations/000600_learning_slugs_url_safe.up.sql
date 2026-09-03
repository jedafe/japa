-- Enforce URL-safe slugs for learning tracks, modules, and lessons.
-- Bad slugs (spaces, uppercase, special characters) break cross-app links.

ALTER TABLE learning_tracks
    ADD CONSTRAINT learning_tracks_slug_url_safe
    CHECK (slug ~ '^[a-z0-9-]+$');

ALTER TABLE learning_modules
    ADD CONSTRAINT learning_modules_slug_url_safe
    CHECK (slug ~ '^[a-z0-9-]+$');

ALTER TABLE learning_lessons
    ADD CONSTRAINT learning_lessons_slug_url_safe
    CHECK (slug ~ '^[a-z0-9-]+$');
