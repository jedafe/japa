-- 000510_comment_votes_pk_conformance.down.sql
-- Reverses 000510: restores the id column and reverts to the original
-- migration 000103 schema (id uuid PRIMARY KEY + UNIQUE(comment_id, user_id)).

BEGIN;

ALTER TABLE comment_votes DROP CONSTRAINT IF EXISTS comment_votes_pkey;

ALTER TABLE comment_votes
    ADD COLUMN IF NOT EXISTS id uuid PRIMARY KEY DEFAULT gen_random_uuid();

ALTER TABLE comment_votes
    ADD CONSTRAINT comment_votes_comment_id_user_id_key
    UNIQUE (comment_id, user_id);

COMMIT;
