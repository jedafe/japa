-- 000973_vote_revert_and_spam_flag_rules.up.sql
-- ADR-136 residual fixes: vote-revert rules let repeated vote flips and
-- unvotes converge the author's balance to the current vote value
-- (reaction.reverted undoes a positive award; reaction.negative.reverted
-- undoes a penalty — keep magnitudes mirrored with reaction.added /
-- reaction.negative). spam.flagged is the auto-flag signal, seeded
-- DISABLED so only confirmed blocks (spam.detected) penalize by default.

BEGIN;

INSERT INTO activity_point_rules (activity_key, point_value, is_enabled, cap_per_day, cap_per_week, description, one_time) VALUES
    ('reaction.reverted',          -1, true,  NULL, NULL, 'Undoes a positive-reaction award when the reaction is removed or flipped', false),
    ('reaction.negative.reverted',  1, true,  NULL, NULL, 'Undoes a negative-reaction penalty when the downvote is removed or flipped', false),
    ('spam.flagged',               -5, false, NULL, NULL, 'Penalty when content is auto-flagged for review (opt-in; blocks use spam.detected)', false)
ON CONFLICT (activity_key) DO NOTHING;

COMMIT;
