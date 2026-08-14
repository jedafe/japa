-- Revert forum poll multi-select support (BUG-002 fix rollback).

ALTER TABLE forum_poll_votes
    DROP CONSTRAINT IF EXISTS forum_poll_votes_option_id_user_id_key;

ALTER TABLE forum_poll_votes
    ADD CONSTRAINT forum_poll_votes_poll_id_user_id_key
    UNIQUE (poll_id, user_id);
