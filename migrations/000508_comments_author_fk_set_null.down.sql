-- 000508_comments_author_fk_set_null.down.sql
-- Reverses 000508: restores comments.author_user_id FK to CASCADE + NOT NULL.
-- Note: existing NULL author_user_id rows (from SET NULL erasure) will need
-- to be handled before re-adding NOT NULL.

ALTER TABLE comments DROP CONSTRAINT IF EXISTS comments_author_user_id_fkey;
ALTER TABLE comments
    ADD CONSTRAINT comments_author_user_id_fkey
    FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE comments ALTER COLUMN author_user_id SET NOT NULL;
