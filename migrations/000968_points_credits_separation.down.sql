-- 000968_points_credits_separation.down.sql
-- Restores the conversion-rate column, the points-tip columns, the retired
-- settings keys and permission, and removes the newly seeded rules. Column
-- values are NOT restored — the data is gone after the up migration.

BEGIN;

ALTER TABLE wallet_settings ADD COLUMN IF NOT EXISTS points_to_credits_rate integer DEFAULT 100;
ALTER TABLE tip_settings ADD COLUMN IF NOT EXISTS accept_points boolean DEFAULT true NOT NULL;
ALTER TABLE tips ADD COLUMN IF NOT EXISTS points_amount bigint DEFAULT 0 NOT NULL;

INSERT INTO permissions (key, description) VALUES
    ('wallet.points.convert', 'Convert points to credits')
ON CONFLICT (key) DO NOTHING;

DELETE FROM activity_point_rules WHERE activity_key IN (
    'user.registered', 'daily.login', 'profile.completed', 'referral.completed',
    'reaction.added', 'academy.lesson_completed', 'academy.track_completed',
    'forum.solution_marked', 'tip.sent', 'reaction.negative', 'spam.detected',
    'content.flagged_removed', 'report.upheld')
AND description IN (
    'Points for creating an account', 'Points for logging in (once per UTC day)',
    'Points for completing profile setup', 'Points when a referral completes',
    'Points when your content receives a positive reaction', 'Points for completing an academy lesson',
    'Points for completing an academy track', 'Points when your reply is marked the solution',
    'Points for tipping a creator (sender earns)', 'Penalty when your content receives a downvote',
    'Penalty when your content is flagged or blocked as spam', 'Penalty when your content is removed by moderation',
    'Penalty when a report against you is upheld');

COMMIT;
