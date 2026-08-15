-- 000494_content_status_in_review.down.sql
-- Reverse the §14 alignment: drop the CHECK constraints and rename
-- 'in_review' rows back to 'pending'. NOTE: any rows inserted as
-- 'in_review' by application code after the up migration will also be
-- renamed to 'pending' here, which is the closest reversible approximation.

-- 1. Drop the CHECK constraints added by the up migration.
ALTER TABLE contents DROP CONSTRAINT IF EXISTS contents_status_check;
ALTER TABLE contents DROP CONSTRAINT IF EXISTS contents_visibility_check;

-- 2. Rename 'in_review' rows back to 'pending'.
UPDATE contents SET status = 'pending' WHERE status = 'in_review';
