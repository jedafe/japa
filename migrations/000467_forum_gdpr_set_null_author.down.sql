-- Revert: change forum author FK back to CASCADE + NOT NULL (BUG-010 rollback).
-- WARNING: rolling back after GDPR erasure has run will orphan anonymized
-- threads/replies (author_user_id NULL). Only run this rollback if no
-- erasure has occurred since migration 000467 was applied.

ALTER TABLE forum_threads DROP CONSTRAINT IF EXISTS forum_threads_author_user_id_fkey;
ALTER TABLE forum_threads
    ADD CONSTRAINT forum_threads_author_user_id_fkey
    FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE forum_threads ALTER COLUMN author_user_id SET NOT NULL;

ALTER TABLE forum_replies DROP CONSTRAINT IF EXISTS forum_replies_author_user_id_fkey;
ALTER TABLE forum_replies
    ADD CONSTRAINT forum_replies_author_user_id_fkey
    FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE forum_replies ALTER COLUMN author_user_id SET NOT NULL;
