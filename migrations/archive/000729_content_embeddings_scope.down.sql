-- 000729_content_embeddings_scope.down.sql

DROP INDEX IF EXISTS idx_content_embeddings_scope;

ALTER TABLE content_embeddings
    DROP CONSTRAINT IF EXISTS content_embeddings_visibility_chk;

ALTER TABLE content_embeddings
    DROP COLUMN IF EXISTS visibility,
    DROP COLUMN IF EXISTS space_id,
    DROP COLUMN IF EXISTS author_id;
