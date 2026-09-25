-- Section 36: Learning Platform — preview lessons.
-- Adds is_preview to learning_lessons so a lesson can be marked as freely
-- previewable without enrollment (ADR-060 lesson-player access check:
-- 403 if not enrolled AND not is_preview). NOT NULL with DEFAULT false is a
-- metadata-only change on PostgreSQL 18 (no table rewrite), safe on hot tables.

ALTER TABLE learning_lessons
    ADD COLUMN IF NOT EXISTS is_preview boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN learning_lessons.is_preview IS 'True when the lesson can be previewed without enrollment (ADR-060 access gate).';
