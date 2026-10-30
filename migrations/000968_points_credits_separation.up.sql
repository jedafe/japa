-- 000968_points_credits_separation.up.sql
-- Points/Credits mutual exclusivity (ADR-129): Points are a pure
-- activity/reputation currency and Credits a pure transactional currency —
-- no conversion exists in either direction. Drops the conversion-rate
-- column, the points-as-tip-method columns, the conversion permission and
-- retired system_settings keys, and seeds the signed activity_point_rules
-- covering the full site activity surface.
--
-- Ref: .jeos/sections/28-wallet-credits.md, .jeos/sections/125-donations-tips.md

BEGIN;

ALTER TABLE wallet_settings DROP COLUMN IF EXISTS points_to_credits_rate;
ALTER TABLE tip_settings DROP COLUMN IF EXISTS accept_points;

-- Preserve historical point-tip amounts for audit before dropping the
-- column — the points value itself is lost data otherwise (ADR-136).
CREATE TABLE IF NOT EXISTS tips_points_legacy AS
SELECT id, sender_user_id, recipient_type, recipient_user_id,
       recipient_organization_id, points_amount, created_at
FROM tips WHERE method = 'points' AND points_amount > 0;

ALTER TABLE tips DROP COLUMN IF EXISTS points_amount;

DELETE FROM system_settings WHERE key IN (
    'wallet.points_to_credit_rate', 'wallet.min_convert_points',
    'tips.points_daily_limit', 'tips.points_daily_recipient_limit');

DELETE FROM role_permissions WHERE permission_id IN
    (SELECT id FROM permissions WHERE key = 'wallet.points.convert');
DELETE FROM permissions WHERE key = 'wallet.points.convert';

-- New activity rules covering the full activity surface. Negative
-- point_value = penalty. Admins retune freely via /admin/wallet/rules.
INSERT INTO activity_point_rules (activity_key, point_value, is_enabled, cap_per_day, cap_per_week, description, one_time) VALUES
    ('user.registered',          10,  true, NULL, NULL, 'Points for creating an account', true),
    ('daily.login',               1,  true, 1,    NULL, 'Points for logging in (once per UTC day)', false),
    ('profile.completed',        15,  true, NULL, NULL, 'Points for completing profile setup', true),
    ('referral.completed',       25,  true, NULL, NULL, 'Points when a referral completes', false),
    ('reaction.added',            1,  true, 50,   NULL, 'Points when your content receives a positive reaction', false),
    ('academy.lesson_completed',  5,  true, NULL, NULL, 'Points for completing an academy lesson', false),
    ('academy.track_completed',  50,  true, NULL, NULL, 'Points for completing an academy track', false),
    ('forum.solution_marked',    10,  true, NULL, NULL, 'Points when your reply is marked the solution', false),
    ('tip.sent',                  5,  true, NULL, NULL, 'Points for tipping a creator (sender earns)', false),
    ('reaction.negative',        -1,  true, NULL, NULL, 'Penalty when your content receives a downvote', false),
    ('spam.detected',           -20,  true, NULL, NULL, 'Penalty when your content is flagged or blocked as spam', false),
    ('content.flagged_removed', -10,  true, NULL, NULL, 'Penalty when your content is removed by moderation', false),
    ('report.upheld',           -10,  true, NULL, NULL, 'Penalty when a report against you is upheld', false)
ON CONFLICT (activity_key) DO NOTHING;

COMMIT;
