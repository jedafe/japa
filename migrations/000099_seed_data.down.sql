-- Phase 05: Reverse seed data migration.
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-BACKEND.md Phase 05
-- Removes seed data inserted by 000099_seed_data.up.sql.
-- Does NOT drop tables created by earlier migrations — only seed rows.

-- Delete in reverse dependency order (children before parents).

-- Page templates
DELETE FROM pagebuilder_templates
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'blank' AND is_system = true;

-- Languages (translations)
DELETE FROM translations
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND "key" LIKE 'lang.%';

-- Known bots (added ones)
DELETE FROM known_bots
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name IN ('CCBot', 'MJ12bot');

-- Comparison type definitions
DELETE FROM comparison_type_definitions
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('universities', 'visa-types', 'banks', 'courses')
  AND is_system = true;

-- Notification types (added ones)
DELETE FROM notification_types
WHERE key IN (
    'follow.received', 'mention.content', 'mention.comment',
    'content.reaction', 'content.featured', 'content.comment', 'content.reshare',
    'comment.reply', 'comment.vote',
    'forum.solved', 'forum.vote',
    'dm.message', 'dm.request',
    'org.invite', 'org.member_joined', 'claim.approved', 'claim.rejected',
    'review.received',
    'payment.succeeded', 'payment.failed', 'subscription.renewed',
    'subscription.cancelled', 'subscription.expiring',
    'ban.issued', 'warning.issued', 'appeal.resolved', 'report.actioned',
    'course.completed', 'certificate.issued',
    'ticket.status_update',
    'broadcast'
);

-- Ad slot types
DELETE FROM ad_slot_types
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slot_name IN ('feed_inline', 'sidebar', 'search_sponsored', 'comparison_sponsored', 'forum_inline');

-- Role permissions (all seeded roles)
DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug IN ('super_admin', 'member', 'guest')
);

-- User roles (seeded ones)
DELETE FROM user_roles
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('super_admin', 'member', 'guest');

-- Group role mappings
DELETE FROM group_roles
WHERE group_id IN (
    SELECT id FROM user_groups
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug IN ('super_admin', 'members', 'guest')
);

-- Permissions (all seeded)
DELETE FROM permissions
WHERE key IN (
    'content.create', 'content.edit.own', 'content.edit.any', 'content.delete.own', 'content.delete.any',
    'content.publish', 'content.schedule',
    'blog.create', 'blog.edit.own', 'blog.edit.any', 'blog.publish', 'blog.review',
    'forum.create', 'forum.vote', 'forum.solve', 'forum.poll.create',
    'content.page_builder.create',
    'spaces.create', 'spaces.moderate.assigned', 'spaces.moderate.any', 'spaces.moderator.assign',
    'moderation.report', 'moderation.warn', 'moderation.ban', 'moderation.appeal.review',
    'moderation.content.review', 'moderation.broadcast',
    'media.upload', 'media.delete.own', 'media.delete.any',
    'reactions.create', 'reactions.delete.own',
    'bookmarks.create', 'bookmarks.manage',
    'tags.create', 'tags.manage',
    'follows.create',
    'notifications.manage',
    'dm.send', 'dm.request',
    'wallet.view', 'wallet.convert',
    'payments.create', 'payments.view.own',
    'learning.enroll', 'learning.manage',
    'tickets.create', 'tickets.manage',
    'webhooks.manage',
    'organizations.create', 'organizations.manage',
    'professional.create', 'professional.manage',
    'search.analytics.view',
    'analytics.view.own', 'analytics.view.all',
    'api.keys.self_manage', 'api.keys.view.all', 'api.keys.manage', 'api.keys.analytics.view',
    'docs.manage',
    'affiliate.enroll', 'affiliate.manage',
    'ai.context.points_balance', 'ai.context.course_progress', 'ai.context.mentorship_activity',
    'ai.context.forum_activity', 'ai.context.referral_activity', 'ai.context.moderation_history',
    'admin.settings.manage', 'admin.users.manage', 'admin.groups.manage',
    'admin.spaces.manage', 'admin.feature_flags.manage', 'admin.i18n.manage'
);

-- System settings (seeded ones)
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'auth.password.min_length', 'auth.password.require_uppercase', 'auth.session.refresh_days',
    'auth.totp.required_for_admin', 'auth.email.verify_required',
    'feed.fanout_threshold', 'feed.velocity.gravity', 'feed.max_followed_spaces_in_feed',
    'content.edit_window_minutes', 'comments.edit_window_minutes', 'content.max_tags',
    'spaces.bubble_depth', 'spaces.max_pinned_posts',
    'moderation.auto_hide_report_count', 'moderation.ai_auto_flag_threshold',
    'wallet.min_convert_points', 'wallet.points_to_credit_rate', 'wallet.max_daily_earn_points',
    'api.rate_limit.basic', 'api.rate_limit.standard', 'api.rate_limit.premium',
    'api.keys.max_per_user',
    'ads.frequency_cap_per_day', 'ads.min_campaign_budget',
    'ai.monthly_budget_usd', 'ai.japabot_enabled', 'ai.content_assist_enabled',
    'media.storage.provider', 'media.storage.endpoint', 'media.storage.bucket',
    'media.storage.region', 'media.storage.access_key', 'media.storage.secret_key',
    'media.cdn.base_url', 'media.max_upload_mb', 'media.allowed_video_types',
    'tickets.default_sla_hours', 'tickets.auto_close_resolved_days',
    'audit.log_retention_days',
    'i18n.default_language', 'i18n.enabled_languages',
    'gdpr.deletion_grace_days', 'gdpr.export_expiry_days',
    'affiliate.min_account_days', 'affiliate.attribution_window_days',
    'affiliate.commission.pro_monthly', 'affiliate.commission.pro_annual',
    'affiliate.commission.course', 'affiliate.max_links_per_user'
  );
