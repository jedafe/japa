-- Reverse of 000267: drop the is_preview column from learning_lessons.

ALTER TABLE learning_lessons
    DROP COLUMN IF EXISTS is_preview;
