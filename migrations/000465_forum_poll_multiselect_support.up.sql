-- Forum poll multi-select support (BUG-002 fix).
-- Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Polls)
--
-- The original UNIQUE(poll_id, user_id) constraint enforced one vote per user
-- per poll regardless of the is_multi_select flag, making multi-select polls
-- impossible. Replace it with UNIQUE(option_id, user_id) so:
--   - single-select: one vote per option per user (app layer deletes prior
--     vote before inserting a new one)
--   - multi-select: one vote per option per user (multiple options allowed)

ALTER TABLE forum_poll_votes
    DROP CONSTRAINT IF EXISTS forum_poll_votes_poll_id_user_id_key;

ALTER TABLE forum_poll_votes
    ADD CONSTRAINT forum_poll_votes_option_id_user_id_key
    UNIQUE (option_id, user_id);
