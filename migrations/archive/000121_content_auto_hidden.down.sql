DROP INDEX IF EXISTS idx_contents_auto_hidden;

ALTER TABLE contents
    DROP COLUMN IF EXISTS auto_hidden_at,
    DROP COLUMN IF EXISTS auto_hidden;
