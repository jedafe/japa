-- GDPR: change forum_threads and forum_replies author FK from CASCADE to
-- SET NULL so content survives user erasure (ADR-014: never hard delete;
-- soft-delete-and-anonymize). The GDPR service anonymizes the body and
-- sets is_anonymous=true before HardDeleteUser nulls the author FK.
-- Ref: japaconnect-engineering/sections/15-gdpr.md + ADR-014 (BUG-010 fix).

-- forum_threads.author_user_id: drop NOT NULL, change FK to SET NULL.
ALTER TABLE forum_threads ALTER COLUMN author_user_id DROP NOT NULL;
ALTER TABLE forum_threads DROP CONSTRAINT IF EXISTS forum_threads_author_user_id_fkey;
ALTER TABLE forum_threads
    ADD CONSTRAINT forum_threads_author_user_id_fkey
    FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE SET NULL;

-- forum_replies.author_user_id: drop NOT NULL, change FK to SET NULL.
ALTER TABLE forum_replies ALTER COLUMN author_user_id DROP NOT NULL;
ALTER TABLE forum_replies DROP CONSTRAINT IF EXISTS forum_replies_author_user_id_fkey;
ALTER TABLE forum_replies
    ADD CONSTRAINT forum_replies_author_user_id_fkey
    FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE SET NULL;
