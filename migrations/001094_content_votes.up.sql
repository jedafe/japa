-- 001094_content_votes.up.sql
-- Up/down vote surface on contents rows — mirrors the comment_votes table
-- (000735 baseline shape, 000974 updated_at) and the denormalized counter
-- convention from 000644 (vote_score, upvote_count, downvote_count).
-- Powers the VoteRail on the unified thread page.

BEGIN;

CREATE TABLE content_votes (
    content_id uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    value smallint NOT NULL CHECK (value IN (-1, 1)),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz,
    PRIMARY KEY (content_id, user_id)
);

-- Same index conventions as comment_votes (000735 baseline): the PK covers
-- per-content reads; a dedicated user index serves "my votes" lookups.
CREATE INDEX idx_content_votes_content ON content_votes USING btree (content_id);
CREATE INDEX idx_content_votes_user ON content_votes USING btree (user_id);

ALTER TABLE contents
    ADD COLUMN IF NOT EXISTS vote_score integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS upvote_count integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS downvote_count integer NOT NULL DEFAULT 0;

COMMIT;
