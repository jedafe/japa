-- Remove the keywords column added in 000244.
DROP INDEX IF EXISTS idx_spaces_keywords;
ALTER TABLE spaces DROP COLUMN IF EXISTS keywords;
