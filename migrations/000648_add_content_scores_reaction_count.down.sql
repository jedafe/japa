-- Revert reaction_count column addition.

ALTER TABLE content_scores
    DROP COLUMN IF EXISTS reaction_count;
