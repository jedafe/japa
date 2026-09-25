-- 000624_align_activity_rule_keys.up.sql
-- Aligns activity_point_rules.activity_key values with the canonical
-- dot-notation keys emitted into activity_events (§28, §64). The underscore
-- variants predate the §64 emitter wiring and never matched emitted events,
-- so rules like post_created could never award points.
--
-- Each UPDATE is guarded by NOT EXISTS so a tenant that already has the
-- canonical key keeps it (avoids the UNIQUE(tenant_id, activity_key)
-- constraint violation).
-- Ref: japaconnect-engineering/sections/28-wallet-credits.md
--      japaconnect-engineering/sections/64-gamification.md

BEGIN;

UPDATE activity_point_rules r SET activity_key = 'content.created'
WHERE r.activity_key = 'post_created'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'content.created');

UPDATE activity_point_rules r SET activity_key = 'comment.created'
WHERE r.activity_key = 'comment_created'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'comment.created');

UPDATE activity_point_rules r SET activity_key = 'follow.created'
WHERE r.activity_key = 'follow_created'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'follow.created');

UPDATE activity_point_rules r SET activity_key = 'space.joined'
WHERE r.activity_key = 'space_joined'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'space.joined');

UPDATE activity_point_rules r SET activity_key = 'assessment.completed'
WHERE r.activity_key = 'assessment_completed'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'assessment.completed');

UPDATE activity_point_rules r SET activity_key = 'scam_alert.confirmed'
WHERE r.activity_key = 'scam_alert_confirmed'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'scam_alert.confirmed');

UPDATE activity_point_rules r SET activity_key = 'reaction.added'
WHERE r.activity_key = 'reaction_added'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'reaction.added');

UPDATE activity_point_rules r SET activity_key = 'content.flagged_removed'
WHERE r.activity_key = 'content_flagged_removed'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'content.flagged_removed');

UPDATE activity_point_rules r SET activity_key = 'spam.detected'
WHERE r.activity_key = 'spam_detected'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'spam.detected');

UPDATE activity_point_rules r SET activity_key = 'referral.completed'
WHERE r.activity_key = 'referral_completed'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'referral.completed');

UPDATE activity_point_rules r SET activity_key = 'daily.login'
WHERE r.activity_key = 'daily_login'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'daily.login');

UPDATE activity_point_rules r SET activity_key = 'profile.completed'
WHERE r.activity_key = 'profile_completed'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'profile.completed');

COMMIT;
