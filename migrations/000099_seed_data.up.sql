-- Phase 05: Registry/config seed data.
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-BACKEND.md Phase 05
-- All inserts use ON CONFLICT DO NOTHING for idempotency.
-- Default tenant UUID: '00000000-0000-0000-0000-000000000001'

-- ============================================================
-- 1. System Settings (49 keys from system-settings-catalog.md)
-- ============================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    -- Authentication
    ('00000000-0000-0000-0000-000000000001', 'auth.password.min_length', '"8"'),
    ('00000000-0000-0000-0000-000000000001', 'auth.password.require_uppercase', '"false"'),
    ('00000000-0000-0000-0000-000000000001', 'auth.session.refresh_days', '"30"'),
    ('00000000-0000-0000-0000-000000000001', 'auth.totp.required_for_admin', '"false"'),
    ('00000000-0000-0000-0000-000000000001', 'auth.email.verify_required', '"true"'),
    -- Feed & Content
    ('00000000-0000-0000-0000-000000000001', 'feed.fanout_threshold', '"5000"'),
    ('00000000-0000-0000-0000-000000000001', 'feed.velocity.gravity', '"1.8"'),
    ('00000000-0000-0000-0000-000000000001', 'feed.max_followed_spaces_in_feed', '"50"'),
    ('00000000-0000-0000-0000-000000000001', 'content.edit_window_minutes', '"1440"'),
    ('00000000-0000-0000-0000-000000000001', 'comments.edit_window_minutes', '"60"'),
    ('00000000-0000-0000-0000-000000000001', 'content.max_tags', '"5"'),
    -- Spaces
    ('00000000-0000-0000-0000-000000000001', 'spaces.bubble_depth', '"2"'),
    ('00000000-0000-0000-0000-000000000001', 'spaces.max_pinned_posts', '"5"'),
    -- Moderation
    ('00000000-0000-0000-0000-000000000001', 'moderation.auto_hide_report_count', '"5"'),
    ('00000000-0000-0000-0000-000000000001', 'moderation.ai_auto_flag_threshold', '"0.85"'),
    -- Wallet
    ('00000000-0000-0000-0000-000000000001', 'wallet.min_convert_points', '"100"'),
    ('00000000-0000-0000-0000-000000000001', 'wallet.points_to_credit_rate', '"0.01"'),
    ('00000000-0000-0000-0000-000000000001', 'wallet.max_daily_earn_points', '"500"'),
    -- API Rate Limits
    ('00000000-0000-0000-0000-000000000001', 'api.rate_limit.basic', '"100"'),
    ('00000000-0000-0000-0000-000000000001', 'api.rate_limit.standard', '"1000"'),
    ('00000000-0000-0000-0000-000000000001', 'api.rate_limit.premium', '"10000"'),
    ('00000000-0000-0000-0000-000000000001', 'api.keys.max_per_user', '"5"'),
    -- Ads
    ('00000000-0000-0000-0000-000000000001', 'ads.frequency_cap_per_day', '"3"'),
    ('00000000-0000-0000-0000-000000000001', 'ads.min_campaign_budget', '"10"'),
    -- AI
    ('00000000-0000-0000-0000-000000000001', 'ai.monthly_budget_usd', '"100"'),
    ('00000000-0000-0000-0000-000000000001', 'ai.japabot_enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'ai.content_assist_enabled', '"true"'),
    -- Media
    ('00000000-0000-0000-0000-000000000001', 'media.storage.provider', '"seaweedfs"'),
    ('00000000-0000-0000-0000-000000000001', 'media.storage.endpoint', '""'),
    ('00000000-0000-0000-0000-000000000001', 'media.storage.bucket', '"japaconnect"'),
    ('00000000-0000-0000-0000-000000000001', 'media.storage.region', '"us-east-1"'),
    ('00000000-0000-0000-0000-000000000001', 'media.storage.access_key', '""'),
    ('00000000-0000-0000-0000-000000000001', 'media.storage.secret_key', '""'),
    ('00000000-0000-0000-0000-000000000001', 'media.cdn.base_url', '""'),
    ('00000000-0000-0000-0000-000000000001', 'media.max_upload_mb', '"50"'),
    ('00000000-0000-0000-0000-000000000001', 'media.allowed_video_types', '"mp4,webm,mov"'),
    -- Tickets
    ('00000000-0000-0000-0000-000000000001', 'tickets.default_sla_hours', '"48"'),
    ('00000000-0000-0000-0000-000000000001', 'tickets.auto_close_resolved_days', '"7"'),
    -- Audit
    ('00000000-0000-0000-0000-000000000001', 'audit.log_retention_days', '"365"'),
    -- i18n
    ('00000000-0000-0000-0000-000000000001', 'i18n.default_language', '"en"'),
    ('00000000-0000-0000-0000-000000000001', 'i18n.enabled_languages', '"en,fr"'),
    -- GDPR
    ('00000000-0000-0000-0000-000000000001', 'gdpr.deletion_grace_days', '"30"'),
    ('00000000-0000-0000-0000-000000000001', 'gdpr.export_expiry_days', '"7"'),
    -- Affiliate Program
    ('00000000-0000-0000-0000-000000000001', 'affiliate.min_account_days', '"30"'),
    ('00000000-0000-0000-0000-000000000001', 'affiliate.attribution_window_days', '"30"'),
    ('00000000-0000-0000-0000-000000000001', 'affiliate.commission.pro_monthly', '"15"'),
    ('00000000-0000-0000-0000-000000000001', 'affiliate.commission.pro_annual', '"20"'),
    ('00000000-0000-0000-0000-000000000001', 'affiliate.commission.course', '"10"'),
    ('00000000-0000-0000-0000-000000000001', 'affiliate.max_links_per_user', '"10"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- ============================================================
-- 2. Permission Nodes (comprehensive per Section 03 + amendments)
-- ============================================================
INSERT INTO permissions (key, description) VALUES
    -- Content
    ('content.create', 'Create content (posts, discussions, pages)'),
    ('content.edit.own', 'Edit own content'),
    ('content.edit.any', 'Edit any content'),
    ('content.delete.own', 'Delete own content'),
    ('content.delete.any', 'Delete any content'),
    ('content.publish', 'Publish content'),
    ('content.schedule', 'Schedule content for future publish'),
    -- Blog
    ('blog.create', 'Create blog posts'),
    ('blog.edit.own', 'Edit own blog posts'),
    ('blog.edit.any', 'Edit any blog posts'),
    ('blog.publish', 'Publish blog posts'),
    ('blog.review', 'Review blog submissions'),
    -- Forum
    ('forum.create', 'Create forum discussions'),
    ('forum.vote', 'Vote on forum posts'),
    ('forum.solve', 'Mark forum posts as solved'),
    ('forum.poll.create', 'Create polls in forum'),
    -- Pages
    ('content.page_builder.create', 'Create pages via page builder'),
    -- Spaces
    ('spaces.create', 'Create custom spaces'),
    ('spaces.moderate.assigned', 'Moderate assigned spaces'),
    ('spaces.moderate.any', 'Moderate any space'),
    ('spaces.moderator.assign', 'Assign space moderators'),
    -- Moderation
    ('moderation.report', 'Report content or users'),
    ('moderation.warn', 'Issue user warnings'),
    ('moderation.ban', 'Ban users'),
    ('moderation.appeal.review', 'Review moderation appeals'),
    ('moderation.content.review', 'Review flagged content'),
    ('moderation.broadcast', 'Send broadcast notifications'),
    -- Media
    ('media.upload', 'Upload media files'),
    ('media.delete.own', 'Delete own media'),
    ('media.delete.any', 'Delete any media'),
    -- Reactions
    ('reactions.create', 'React to content'),
    ('reactions.delete.own', 'Remove own reactions'),
    -- Bookmarks
    ('bookmarks.create', 'Create bookmarks'),
    ('bookmarks.manage', 'Manage bookmark folders'),
    -- Tags
    ('tags.create', 'Create tags'),
    ('tags.manage', 'Manage tags and tag moderators'),
    -- Follows
    ('follows.create', 'Follow users and spaces'),
    -- Notifications
    ('notifications.manage', 'Manage notification preferences'),
    -- DMs
    ('dm.send', 'Send direct messages'),
    ('dm.request', 'Initiate DM requests'),
    -- Wallet
    ('wallet.view', 'View wallet balances'),
    ('wallet.convert', 'Convert points to credits'),
    -- Payments
    ('payments.create', 'Create payment intents'),
    ('payments.view.own', 'View own payment history'),
    -- Learning
    ('learning.enroll', 'Enroll in learning tracks'),
    ('learning.manage', 'Manage learning tracks and content'),
    -- Tickets
    ('tickets.create', 'Create support tickets'),
    ('tickets.manage', 'Manage all tickets'),
    -- Webhooks
    ('webhooks.manage', 'Manage webhooks'),
    -- Organizations
    ('organizations.create', 'Create organizations'),
    ('organizations.manage', 'Manage organization members'),
    -- Professional
    ('professional.create', 'Create professional profiles'),
    ('professional.manage', 'Manage professional profiles'),
    -- Search
    ('search.analytics.view', 'View search analytics'),
    -- Analytics
    ('analytics.view.own', 'View own analytics'),
    ('analytics.view.all', 'View all analytics'),
    -- API Keys
    ('api.keys.self_manage', 'Create and revoke own API keys'),
    ('api.keys.view.all', 'View all API keys'),
    ('api.keys.manage', 'Manage all API keys (suspend, revoke, tier)'),
    ('api.keys.analytics.view', 'View per-key API usage analytics'),
    -- Docs
    ('docs.manage', 'Manage documentation pages'),
    -- Affiliate
    ('affiliate.enroll', 'Enroll in affiliate program'),
    ('affiliate.manage', 'Manage affiliate program'),
    -- AI Context
    ('ai.context.points_balance', 'Access points balance in AI context'),
    ('ai.context.course_progress', 'Access course progress in AI context'),
    ('ai.context.mentorship_activity', 'Access mentorship activity in AI context'),
    ('ai.context.forum_activity', 'Access forum activity in AI context'),
    ('ai.context.referral_activity', 'Access referral activity in AI context'),
    ('ai.context.moderation_history', 'Access moderation history in AI context'),
    -- Admin
    ('admin.settings.manage', 'Manage platform settings'),
    ('admin.users.manage', 'Manage user accounts'),
    ('admin.groups.manage', 'Manage user groups and roles'),
    ('admin.spaces.manage', 'Manage all spaces'),
    ('admin.feature_flags.manage', 'Toggle feature flags'),
    ('admin.i18n.manage', 'Manage translations and languages')
ON CONFLICT (key) DO NOTHING;

-- ============================================================
-- 3. User Roles (default roles per system group)
-- ============================================================
INSERT INTO user_roles (tenant_id, name, slug) VALUES
    ('00000000-0000-0000-0000-000000000001', 'Super Admin', 'super_admin'),
    ('00000000-0000-0000-0000-000000000001', 'Member', 'member'),
    ('00000000-0000-0000-0000-000000000001', 'Guest', 'guest')
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- 4. Group-Role Mappings (map system groups to their roles)
-- ============================================================
INSERT INTO group_roles (group_id, role_id)
SELECT ug.id, ur.id
FROM user_groups ug, user_roles ur
WHERE ug.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ug.slug = 'super_admin' AND ur.slug = 'super_admin'
ON CONFLICT DO NOTHING;

INSERT INTO group_roles (group_id, role_id)
SELECT ug.id, ur.id
FROM user_groups ug, user_roles ur
WHERE ug.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ug.slug = 'members' AND ur.slug = 'member'
ON CONFLICT DO NOTHING;

INSERT INTO group_roles (group_id, role_id)
SELECT ug.id, ur.id
FROM user_groups ug, user_roles ur
WHERE ug.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ug.slug = 'guest' AND ur.slug = 'guest'
ON CONFLICT DO NOTHING;

-- ============================================================
-- 5. Role-Permission Mappings
-- ============================================================

-- Super Admin gets ALL permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
ON CONFLICT DO NOTHING;

-- Member role permissions (standard member capabilities)
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key IN (
    'content.create', 'content.edit.own', 'content.delete.own', 'content.publish',
    'blog.create', 'blog.edit.own',
    'forum.create', 'forum.vote',
    'spaces.create',
    'moderation.report',
    'media.upload', 'media.delete.own',
    'reactions.create', 'reactions.delete.own',
    'bookmarks.create', 'bookmarks.manage',
    'tags.create',
    'follows.create',
    'notifications.manage',
    'dm.send', 'dm.request',
    'wallet.view', 'wallet.convert',
    'payments.create', 'payments.view.own',
    'learning.enroll',
    'tickets.create',
    'organizations.create',
    'professional.create',
    'analytics.view.own',
    'api.keys.self_manage'
  )
ON CONFLICT DO NOTHING;

-- Guest role permissions (minimal — read-only with limited interaction)
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'guest'
  AND p.key IN (
    'moderation.report'
  )
ON CONFLICT DO NOTHING;

-- ============================================================
-- 6. Ad Slot Types (5 default types from Section 25)
-- ============================================================
INSERT INTO ad_slot_types (tenant_id, slot_name, description, max_display_per_page) VALUES
    ('00000000-0000-0000-0000-000000000001', 'feed_inline', 'In-feed content ad between posts', 2),
    ('00000000-0000-0000-0000-000000000001', 'sidebar', 'Sidebar display ad', 1),
    ('00000000-0000-0000-0000-000000000001', 'search_sponsored', 'Sponsored result in search listings', 3),
    ('00000000-0000-0000-0000-000000000001', 'comparison_sponsored', 'Featured row in comparison tables', 2),
    ('00000000-0000-0000-0000-000000000001', 'forum_inline', 'In-forum ad between thread posts', 2)
ON CONFLICT (tenant_id, slot_name) DO NOTHING;

-- ============================================================
-- 7. Complete Notification Types (31 types from Section 35)
-- ============================================================
-- The initial migration seeded 8 basic types; add the full catalog here.
INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push) VALUES
    -- Social
    ('follow.received',        'New Follower',             'Someone followed you',                 true,  false, false),
    ('mention.content',        'Mentioned in Post',        '@mentioned in a post',                 true,  false, true),
    ('mention.comment',        'Mentioned in Comment',     '@mentioned in a comment',              true,  false, true),
    -- Content engagement
    ('content.reaction',       'Post Reaction',            'Someone reacted to your post',         true,  false, false),
    ('content.featured',       'Post Featured',            'Your post was featured',               true,  true,  true),
    ('content.comment',        'New Comment',              'New comment on your post',             true,  false, false),
    ('content.reshare',        'Post Reshared',            'Someone reshared your post',           true,  false, false),
    -- Comments
    ('comment.reply',          'Reply to Comment',         'Someone replied to your comment',      true,  false, true),
    ('comment.vote',           'Comment Upvoted',          'Your comment received upvotes',        false, false, false),
    -- Forum
    ('forum.solved',           'Post Marked Solved',       'Your forum post was marked solved',    true,  false, false),
    ('forum.vote',             'Post Voted',               'Your discussion received votes',       false, false, false),
    -- DMs
    ('dm.message',             'New DM',                   'New direct message',                   true,  false, true),
    ('dm.request',             'Message Request',          'Someone wants to message you',         true,  false, true),
    -- Organizations & Profiles
    ('org.invite',             'Org Invitation',           'Invited to join an organization',      true,  true,  true),
    ('org.member_joined',      'New Org Member',           'Someone joined your organization',     true,  false, false),
    ('claim.approved',         'Claim Approved',           'Ownership claim approved',             true,  true,  true),
    ('claim.rejected',         'Claim Rejected',           'Ownership claim rejected',             true,  true,  false),
    -- Reviews
    ('review.received',        'New Review',               'Someone reviewed your profile/org',    true,  false, false),
    -- Payments & Subscriptions
    ('payment.succeeded',      'Payment Succeeded',        'Your payment was processed',           true,  true,  false),
    ('payment.failed',         'Payment Failed',           'Payment could not be processed',       true,  true,  true),
    ('subscription.renewed',   'Subscription Renewed',     'Your subscription was renewed',        false, true,  false),
    ('subscription.cancelled', 'Subscription Cancelled',   'Your subscription was cancelled',      true,  true,  false),
    ('subscription.expiring',  'Subscription Expiring',    'Subscription expiring in 7 days',      false, true,  true),
    -- Moderation
    ('ban.issued',             'Account Banned',           'Your account has been restricted',     true,  true,  true),
    ('warning.issued',         'Account Warning',          'You received a moderation warning',    true,  true,  false),
    ('appeal.resolved',        'Appeal Resolved',          'Your appeal has been reviewed',        true,  true,  false),
    ('report.actioned',        'Report Actioned',          'A report you filed was actioned',      true,  false, false),
    -- Learning
    ('course.completed',       'Course Completed',         'You completed a course',               true,  true,  false),
    ('certificate.issued',     'Certificate Ready',        'Your certificate is ready to download',true,  true,  true),
    -- Tickets
    ('ticket.status_update',   'Ticket Updated',           'Your support ticket was updated',      true,  true,  false),
    -- Admin broadcasts
    ('broadcast',              'Platform Announcement',    'Message from the JapaConnect team',    true,  true,  false)
ON CONFLICT (key) DO NOTHING;

-- ============================================================
-- 8. Comparison Type Definitions + Dimensions (Section 30)
-- ============================================================
-- Universities comparison type (links to university_fields custom field group)
INSERT INTO comparison_type_definitions (tenant_id, name, slug, description, field_group_id, item_label, is_system)
SELECT '00000000-0000-0000-0000-000000000001',
       'Universities', 'universities', 'Compare universities across key dimensions',
       cfg.id, 'University', true
FROM custom_field_groups cfg
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'university_fields'
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO comparison_type_definitions (tenant_id, name, slug, description, field_group_id, item_label, is_system)
SELECT '00000000-0000-0000-0000-000000000001',
       'Visa Types', 'visa-types', 'Compare visa requirements and processing',
       cfg.id, 'Visa', true
FROM custom_field_groups cfg
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'organization_fields'
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO comparison_type_definitions (tenant_id, name, slug, description, field_group_id, item_label, is_system)
SELECT '00000000-0000-0000-0000-000000000001',
       'Banks', 'banks', 'Compare banking services for diaspora',
       cfg.id, 'Bank', true
FROM custom_field_groups cfg
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'organization_fields'
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO comparison_type_definitions (tenant_id, name, slug, description, field_group_id, item_label, is_system)
SELECT '00000000-0000-0000-0000-000000000001',
       'Courses', 'courses', 'Compare learning courses and programs',
       cfg.id, 'Course', true
FROM custom_field_groups cfg
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'university_fields'
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- 9. Known Bots — add missing bots (CCBot, MJ12bot)
-- ============================================================
INSERT INTO known_bots (tenant_id, name, user_agent_pattern, bot_type, policy, is_system) VALUES
    ('00000000-0000-0000-0000-000000000001', 'CCBot', 'CCBot', 'scraper', 'allow', true),
    ('00000000-0000-0000-0000-000000000001', 'MJ12bot', 'MJ12bot', 'scraper', 'limit', true)
ON CONFLICT DO NOTHING;

-- ============================================================
-- 10. Languages (en, fr per Section 18)
-- ============================================================
-- Using the translations table schema from migration 000046 (simplified i18n).
-- Language metadata is stored as translation keys with a 'lang.' prefix.
INSERT INTO translations (tenant_id, locale, "key", value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'en', 'lang.en.name', 'English'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'lang.en.native_name', 'English'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'lang.en.enabled', 'true'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'lang.en.is_default', 'true'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'lang.fr.name', 'French'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'lang.fr.native_name', 'Français'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'lang.fr.enabled', 'true'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'lang.fr.is_default', 'false')
ON CONFLICT (tenant_id, locale, "key") DO NOTHING;

-- ============================================================
-- 11. Page Templates (blank system template)
-- ============================================================
INSERT INTO pagebuilder_templates (tenant_id, name, slug, description, page_type, blocks, is_system)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Blank', 'blank', 'Empty page template', 'landing', '[]', true
)
ON CONFLICT (tenant_id, slug) DO NOTHING;
