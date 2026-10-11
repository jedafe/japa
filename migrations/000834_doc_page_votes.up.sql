-- 000834_doc_page_votes.up.sql
-- §44 (2026-10-10 amendment 2): per-user vote record for doc page feedback.
-- Aggregate helpful_yes/helpful_no counters stay on doc_pages (cheap display);
-- this table makes votes idempotent per user and lets voters change their vote.
BEGIN;

CREATE TABLE IF NOT EXISTS doc_page_votes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    page_id    uuid NOT NULL REFERENCES doc_pages (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    vote       text NOT NULL CHECK (vote IN ('up', 'down')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (page_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_doc_page_votes_user ON doc_page_votes (user_id);

COMMIT;
