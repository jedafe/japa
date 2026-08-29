-- Section 35: add category column to notification_types so the UI can group
-- preferences without hardcoded key→category maps (Rule 16).

BEGIN;

ALTER TABLE notification_types ADD COLUMN IF NOT EXISTS category text NOT NULL DEFAULT 'system';

-- Back-fill categories for existing seeded notification types.
-- Categories match the six UI tabs: social, content, messages, account, billing, system.
UPDATE notification_types SET category = CASE key
    -- Social
    WHEN 'new_follower' THEN 'social'
    WHEN 'follow.received' THEN 'social'
    WHEN 'mention' THEN 'social'
    WHEN 'mention.content' THEN 'social'
    WHEN 'mention.comment' THEN 'social'
    -- Content engagement
    WHEN 'post_reply' THEN 'content'
    WHEN 'reaction' THEN 'content'
    WHEN 'content.reaction' THEN 'content'
    WHEN 'content.featured' THEN 'content'
    WHEN 'content.comment' THEN 'content'
    WHEN 'comment' THEN 'content'
    WHEN 'content.reshare' THEN 'content'
    WHEN 'comment.reply' THEN 'content'
    WHEN 'comment.vote' THEN 'content'
    WHEN 'forum.solved' THEN 'content'
    WHEN 'forum.vote' THEN 'content'
    -- Messages
    WHEN 'dm_message' THEN 'messages'
    WHEN 'dm.message' THEN 'messages'
    WHEN 'dm_request' THEN 'messages'
    WHEN 'dm.request' THEN 'messages'
    -- Account / Org / Reviews
    WHEN 'org.invite' THEN 'account'
    WHEN 'org.member_joined' THEN 'account'
    WHEN 'claim.approved' THEN 'account'
    WHEN 'claim.rejected' THEN 'account'
    WHEN 'review.received' THEN 'account'
    -- Billing
    WHEN 'payment.succeeded' THEN 'billing'
    WHEN 'payment.failed' THEN 'billing'
    WHEN 'subscription.renewed' THEN 'billing'
    WHEN 'subscription.cancelled' THEN 'billing'
    WHEN 'subscription.expiring' THEN 'billing'
    -- Learning
    WHEN 'course.completed' THEN 'system'
    WHEN 'certificate.issued' THEN 'system'
    WHEN 'university_instructor_approved' THEN 'system'
    WHEN 'university_course_approved' THEN 'system'
    -- Moderation / Tickets / Broadcasts / System
    WHEN 'ban.issued' THEN 'system'
    WHEN 'warning.issued' THEN 'system'
    WHEN 'appeal.resolved' THEN 'system'
    WHEN 'report.actioned' THEN 'system'
    WHEN 'report.new' THEN 'system'
    WHEN 'scam_alert.confirmed' THEN 'system'
    WHEN 'ticket.status_update' THEN 'system'
    WHEN 'broadcast' THEN 'system'
    WHEN 'system_announcement' THEN 'system'
    WHEN 'course_enrolled' THEN 'system'
    WHEN 'review_approved' THEN 'system'
    WHEN 'referral_qualified' THEN 'system'
    WHEN 'moderation_action' THEN 'system'
    WHEN 'calendar_reminder' THEN 'system'
    ELSE category
END;

COMMIT;
