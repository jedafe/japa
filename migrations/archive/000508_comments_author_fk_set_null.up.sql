-- 000508_comments_author_fk_set_null.up.sql
-- GDPR: change comments.author_user_id FK from CASCADE to SET NULL so
-- comments survive user erasure (Section 49 spec: ON DELETE SET NULL).
-- The GDPR service (AnonymizeCommentsByAuthor) sets body to '[deleted]'
-- and deleted_at = now() before HardDeleteUser nulls the author FK.
-- With CASCADE (migration 000103), the comments were deleted entirely,
-- making the anonymization pointless and destroying conversation context
-- (replies lost their parent via parent_id CASCADE).
-- Ref: japaconnect-engineering/sections/49-comments-replies.md (schema)
--      japaconnect-engineering/sections/15-gdpr.md + ADR-014 (BUG-001 fix).

-- comments.author_user_id: drop NOT NULL, change FK to SET NULL.
ALTER TABLE comments ALTER COLUMN author_user_id DROP NOT NULL;
ALTER TABLE comments DROP CONSTRAINT IF EXISTS comments_author_user_id_fkey;
ALTER TABLE comments
    ADD CONSTRAINT comments_author_user_id_fkey
    FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE SET NULL;
