BEGIN;

DELETE FROM activity_point_rules WHERE activity_key IN
    ('reaction.reverted', 'reaction.negative.reverted', 'spam.flagged')
AND description IN (
    'Undoes a positive-reaction award when the reaction is removed or flipped',
    'Undoes a negative-reaction penalty when the downvote is removed or flipped',
    'Penalty when content is auto-flagged for review (opt-in; blocks use spam.detected)');

COMMIT;
