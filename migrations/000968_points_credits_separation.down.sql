-- 000968_points_credits_separation.down.sql
-- Restores the conversion-rate column, the points-tip columns, the retired
-- settings keys and permission, and removes the newly seeded rules. Column
-- values are NOT restored — the data is gone after the up migration.

BEGIN;

ALTER TABLE wallet_settings ADD COLUMN IF NOT EXISTS points_to_credits_rate integer DEFAULT 100;
ALTER TABLE tip_settings ADD COLUMN IF NOT EXISTS accept_points boolean DEFAULT true NOT NULL;
ALTER TABLE tips ADD COLUMN IF NOT EXISTS points_amount bigint DEFAULT 0 NOT NULL;

-- Restore point-tip amounts from the audit copy created by the up
-- migration, then drop the audit table.
UPDATE tips t SET points_amount = l.points_amount
FROM tips_points_legacy l WHERE t.id = l.id;
DROP TABLE IF EXISTS tips_points_legacy;

INSERT INTO permissions (key, description) VALUES
    ('wallet.points.convert', 'Convert points to credits')
ON CONFLICT (key) DO NOTHING;

-- Restore the member-tier grants and retired system_settings keys
-- (mirrors 000917 + baseline seed values).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM user_roles r
JOIN permissions p ON p.key = 'wallet.points.convert'
WHERE r.slug IN ('member', 'pro_member', 'business_member')
ON CONFLICT DO NOTHING;

INSERT INTO system_settings (key, value) VALUES
    ('wallet.points_to_credit_rate', '"0.001"'),
    ('wallet.min_convert_points', '"100"'),
    ('tips.points_daily_limit', '"500"'),
    ('tips.points_daily_recipient_limit', '"0"')
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
