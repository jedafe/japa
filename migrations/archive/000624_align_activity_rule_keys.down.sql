-- 000624_align_activity_rule_keys.down.sql
-- Reverts the activity_key rename back to the legacy underscore variants.
-- Ref: japaconnect-engineering/sections/28-wallet-credits.md
--      japaconnect-engineering/sections/64-gamification.md

BEGIN;

UPDATE activity_point_rules r SET activity_key = 'post_created'
WHERE r.activity_key = 'content.created'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'post_created');

UPDATE activity_point_rules r SET activity_key = 'comment_created'
WHERE r.activity_key = 'comment.created'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'comment_created');

UPDATE activity_point_rules r SET activity_key = 'follow_created'
WHERE r.activity_key = 'follow.created'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'follow_created');

UPDATE activity_point_rules r SET activity_key = 'space_joined'
WHERE r.activity_key = 'space.joined'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'space_joined');

UPDATE activity_point_rules r SET activity_key = 'assessment_completed'
WHERE r.activity_key = 'assessment.completed'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'assessment_completed');

UPDATE activity_point_rules r SET activity_key = 'scam_alert_confirmed'
WHERE r.activity_key = 'scam_alert.confirmed'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'scam_alert_confirmed');

UPDATE activity_point_rules r SET activity_key = 'reaction_added'
WHERE r.activity_key = 'reaction.added'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'reaction.added');

UPDATE activity_point_rules r SET activity_key = 'content_flagged_removed'
WHERE r.activity_key = 'content.flagged_removed'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'content_flagged_removed');

UPDATE activity_point_rules r SET activity_key = 'spam_detected'
WHERE r.activity_key = 'spam.detected'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'spam_detected');

UPDATE activity_point_rules r SET activity_key = 'referral_completed'
WHERE r.activity_key = 'referral.completed'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'referral_completed');

UPDATE activity_point_rules r SET activity_key = 'daily_login'
WHERE r.activity_key = 'daily.login'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'daily_login');

UPDATE activity_point_rules r SET activity_key = 'profile_completed'
WHERE r.activity_key = 'profile.completed'
  AND NOT EXISTS (SELECT 1 FROM activity_point_rules x WHERE x.tenant_id = r.tenant_id AND x.activity_key = 'profile_completed');

COMMIT;
