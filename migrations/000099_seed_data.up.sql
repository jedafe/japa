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
    ('00000000-0000-0000-0000-000000000001', 'forum.edit_window_minutes', '"30"'),
    ('00000000-0000-0000-0000-000000000001', 'auth.session.access_token_ttl_minutes', '"15"'),
    ('00000000-0000-0000-0000-000000000001', 'security.max_login_attempts', '"5"'),
    ('00000000-0000-0000-0000-000000000001', 'security.fail_window_minutes', '"15"'),
    ('00000000-0000-0000-0000-000000000001', 'notifications.max_delivery_attempts', '"5"'),
    ('00000000-0000-0000-0000-000000000001', 'notifications.batch_window_minutes', '"5"'),
    ('00000000-0000-0000-0000-000000000001', 'moderation.max_links_per_post', '"3"'),
    ('00000000-0000-0000-0000-000000000001', 'moderation.new_account_threshold_days', '"7"'),
    ('00000000-0000-0000-0000-000000000001', 'content.min_length', '"10"'),
    ('00000000-0000-0000-0000-000000000001', 'content.edit_lock_ttl_seconds', '"120"'),
    ('00000000-0000-0000-0000-000000000001', 'api.idempotency_ttl_hours', '"24"'),
    ('00000000-0000-0000-0000-000000000001', 'search.autocomplete_max', '"8"'),
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
-- 2. Permission Nodes (flat taxonomy — matches bootstrapPermissions in server.go)
--    Routes gate on these flat nodes; the .own/.any granularity was superseded.
-- ============================================================
INSERT INTO permissions (key, description) VALUES
    -- Content Core (Section 14)
    ('content.create', 'Create content'),
    ('content.edit', 'Edit content'),
    ('content.delete', 'Delete content'),
    ('content.revisions.view', 'View content revisions'),
    ('content.pin', 'Pin content to spaces'),
    ('content.reaction_types.manage', 'Manage reaction types'),
    ('content.reactions.add', 'Add reactions to content'),
    ('content.reactions.remove', 'Remove reactions from content'),
    ('content.scores.view', 'View content scores'),
    ('content.jsonld.manage', 'Manage JSON-LD templates'),
    ('content.scheduled.view', 'View scheduled content'),
    ('content.publish', 'Publish content'),
    ('content.schedule', 'Schedule content for future publish'),
    -- Page Builder (Section 16)
    ('content.page_builder.create', 'Create pages with page builder'),
    ('content.page_builder.edit', 'Edit pages with page builder'),
    ('content.page_builder.delete', 'Delete page builder pages'),
    ('content.render', 'Render content via TipTap SSR'),
    ('pagebuilder.blocks.manage', 'Manage page builder blocks'),
    ('pagebuilder.templates.manage', 'Manage page builder templates'),
    ('pagebuilder.themes.manage', 'Manage page builder themes'),
    ('pages.manage', 'Manage pages (create, edit, delete)'),
    ('pages.publish', 'Publish and archive pages'),
    -- Custom Fields (Section 06)
    ('custom_fields.group.view', 'View custom field groups'),
    ('custom_fields.group.create', 'Create custom field groups'),
    ('custom_fields.group.edit', 'Edit custom field groups'),
    ('custom_fields.group.delete', 'Delete custom field groups'),
    ('custom_fields.field.create', 'Create custom fields'),
    ('custom_fields.field.edit', 'Edit custom fields'),
    ('custom_fields.field.delete', 'Delete custom fields'),
    ('custom_fields.values.set', 'Set custom field values'),
    -- Forms (Section 33)
    ('forms.create', 'Create forms'),
    ('forms.edit', 'Edit forms'),
    ('forms.delete', 'Delete forms'),
    ('forms.submissions.view', 'View form submissions'),
    ('forms.submissions.edit', 'Edit form submission status'),
    ('forms.submissions.delete', 'Delete form submissions'),
    -- Spaces (Section 05)
    ('spaces.create', 'Create spaces'),
    ('spaces.edit', 'Edit spaces'),
    ('spaces.delete', 'Delete spaces'),
    ('spaces.moderate', 'Moderate spaces (manage rules and pins)'),
    ('spaces.moderate.any', 'Moderate any space'),
    ('spaces.moderate.assigned', 'Assign or remove space moderators'),
    ('spaces.admin', 'Manage space permission presets'),
    -- Space Types (Section 05)
    ('space_types.create', 'Create space types'),
    ('space_types.edit', 'Edit space types'),
    ('space_types.delete', 'Delete space types'),
    -- Tags
    ('tags.create', 'Create general tags'),
    ('tags.edit', 'Edit general tags'),
    ('tags.delete', 'Delete general tags'),
    ('tags.manage', 'Manage tags and tag moderators'),
    -- Forum Q&A (Section 08)
    ('forum.thread.create', 'Create forum threads'),
    ('forum.thread.edit_own', 'Edit own forum threads'),
    ('forum.thread.edit_any', 'Edit any forum thread (admin)'),
    ('forum.thread.delete_own', 'Delete own forum threads'),
    ('forum.thread.delete_any', 'Delete any forum thread (admin)'),
    ('forum.thread.solve', 'Mark forum threads as solved'),
    ('forum.reply.create', 'Create forum replies'),
    ('forum.reply.edit_own', 'Edit own forum replies'),
    ('forum.reply.edit_any', 'Edit any forum reply (admin)'),
    ('forum.reply.delete_own', 'Delete own forum replies'),
    ('forum.reply.delete_any', 'Delete any forum reply (admin)'),
    ('forum.vote', 'Vote on forum threads and replies'),
    ('forum.bookmark.manage', 'Manage forum bookmarks'),
    ('forum.poll.create', 'Create forum polls'),
    ('forum.poll.vote', 'Vote on forum polls'),
    ('forum.tag.create', 'Create forum tags'),
    ('forum.tag.edit', 'Edit forum tags'),
    ('forum.tag.moderate', 'Moderate forum tags'),
    ('forum.admin', 'Admin-only forum management'),
    -- Blog Extension (Section 24)
    ('blog.create', 'Create blog posts'),
    ('blog.edit_own', 'Edit own blog posts'),
    ('blog.edit_any', 'Edit any blog post (admin)'),
    ('blog.publish', 'Publish blog posts'),
    ('blog.feature', 'Feature/unfeature blog posts'),
    ('blog.series.create', 'Create blog series'),
    ('blog.series.edit', 'Edit blog series'),
    ('blog.series.delete', 'Delete blog series'),
    ('blog.review.create', 'Create content reviews'),
    ('blog.review.manage', 'Manage content review status'),
    ('blog.review', 'Review blog submissions'),
    ('blog.rss.view', 'View RSS feed'),
    -- Reviews (Section 29)
    ('reviews.create', 'Create reviews'),
    ('reviews.edit_own', 'Edit own reviews'),
    ('reviews.edit_any', 'Edit any review (admin)'),
    ('reviews.delete_own', 'Delete own reviews'),
    ('reviews.delete_any', 'Delete any review (admin)'),
    ('reviews.flag', 'Flag reviews for moderation'),
    ('reviews.moderate', 'Moderate review status'),
    ('reviews.aggregate.view', 'View review aggregates'),
    -- Comparison System (Section 30)
    ('comparison.type.create', 'Create comparison types'),
    ('comparison.type.edit', 'Edit comparison types'),
    ('comparison.type.delete', 'Delete comparison types'),
    ('comparison.type.access.manage', 'Manage comparison type access'),
    ('comparison.create', 'Create comparisons'),
    ('comparison.edit_own', 'Edit own comparisons'),
    ('comparison.edit_any', 'Edit any comparison (admin)'),
    ('comparison.delete_own', 'Delete own comparisons'),
    ('comparison.delete_any', 'Delete any comparison (admin)'),
    ('comparison.item.manage', 'Manage comparison items'),
    ('comparison.dimension.manage', 'Manage comparison dimensions'),
    ('comparison.value.manage', 'Manage comparison values'),
    -- Professional Profiles (Section 33)
    ('professional_profile.type.create', 'Create professional profile types'),
    ('professional_profile.type.view', 'View professional profile types'),
    ('professional_profile.create', 'Create professional profiles'),
    ('professional_profile.edit_own', 'Edit own professional profiles'),
    ('professional_profile.edit_any', 'Edit any professional profile (admin)'),
    ('professional_profile.delete_own', 'Delete own professional profiles'),
    ('professional_profile.delete_any', 'Delete any professional profile (admin)'),
    ('professional_profile.member.manage', 'Manage professional profile members'),
    ('professional_profile.invite', 'Send professional profile invitations'),
    ('professional_profile.author.switch', 'Switch authorship to professional profile'),
    ('professional.create', 'Create professional profiles'),
    ('professional.manage', 'Manage professional profiles'),
    -- Social Graph (Section 10)
    ('follows.create', 'Follow users and spaces'),
    ('follows.toggle', 'Follow/unfollow entities'),
    ('follows.view', 'View follower/following lists'),
    -- Notifications (Section 12)
    ('notifications.view', 'View notifications'),
    ('notifications.manage', 'Manage notification types'),
    -- DMs (Section 15)
    ('dm.send', 'Send direct messages'),
    ('dm.view', 'View direct messages'),
    ('dm.request', 'Initiate DM requests'),
    ('dm.manage_privacy', 'Manage DM privacy settings'),
    -- Media (Section 13)
    ('media.upload', 'Upload media files'),
    ('media.view_own', 'View own uploaded media'),
    ('media.view_all', 'View all media (admin)'),
    ('media.delete_own', 'Delete own uploaded media'),
    ('media.delete_any', 'Delete any media (admin)'),
    ('media.video.upload', 'Upload video files'),
    ('media.library.view_own', 'View own media library'),
    ('media.library.view_all', 'View all media library (admin)'),
    ('media.quarantine.manage', 'Review and manage quarantined media'),
    -- Reactions
    ('reactions.create', 'React to content'),
    ('reactions.delete.own', 'Remove own reactions'),
    -- Bookmarks
    ('bookmarks.create', 'Create bookmarks'),
    ('bookmarks.manage', 'Manage bookmark folders'),
    -- Wallet & Credits (Section 28)
    ('wallet.points.balance', 'View points balance'),
    ('wallet.credits.balance', 'View credits balance'),
    ('wallet.points.earn', 'Earn points'),
    ('wallet.credits.spend', 'Spend credits'),
    ('wallet.points.convert', 'Convert points to credits'),
    ('wallet.credits.purchase', 'Purchase credits'),
    ('wallet.view', 'View wallet balances'),
    ('wallet.convert', 'Convert points to credits'),
    ('wallet.admin.adjust', 'Admin balance adjustment'),
    ('wallet.admin.rules.manage', 'Manage activity point rules'),
    ('wallet.admin.settings.manage', 'Manage wallet settings'),
    -- Payment Core (Section 17)
    ('payments.create', 'Create payments'),
    ('payments.view_own', 'View own payments'),
    ('payments.view_any', 'View any payment (admin)'),
    ('payments.view.own', 'View own payment history'),
    ('payments.refund', 'Refund payments'),
    ('payments.webhook', 'Handle payment webhooks'),
    ('payments.subscriptions.manage', 'Manage subscriptions'),
    ('payments.discount_codes.manage', 'Manage discount codes'),
    -- Learning Platform (Section 36)
    ('learning.view', 'View published learning tracks'),
    ('learning.progress.update', 'Update lesson progress'),
    ('learning.certificate.issue', 'Issue learning certificates'),
    ('learning.admin.manage', 'Manage learning tracks, modules, and lessons'),
    ('learning.enroll', 'Enroll in learning tracks'),
    ('learning.manage', 'Manage learning tracks and content'),
    -- Tickets (Section 38)
    ('tickets.submit', 'Submit support tickets'),
    ('tickets.view_own', 'View own tickets'),
    ('tickets.create', 'Create support tickets'),
    ('tickets.admin', 'Admin ticket management'),
    ('tickets.type.manage', 'Manage ticket types'),
    ('tickets.manage', 'Manage all tickets'),
    -- Webhooks (Section 23)
    ('webhooks.manage', 'Manage webhooks'),
    ('integrations.webhooks.manage', 'Manage webhook endpoint configurations'),
    -- Organizations (Section 04)
    ('organizations.create', 'Create organizations'),
    ('organizations.manage', 'Manage organization members'),
    -- Search (Section 09)
    ('search.analytics.view', 'View search analytics'),
    -- Analytics (Section 40)
    ('analytics.view.own', 'View own analytics'),
    ('analytics.view.all', 'View all analytics'),
    ('analytics.admin', 'View analytics dashboard and security scan results'),
    ('analytics.cache.flush', 'Flush platform caches (admin-only)'),
    ('admin.analytics.search', 'Admin analytics search'),
    -- API Keys (Section 47)
    ('api.keys.self_manage', 'Create and revoke own API keys'),
    ('api.keys.view.all', 'View all API keys'),
    ('api.keys.manage', 'Manage all API keys (suspend, revoke, tier)'),
    ('api.keys.analytics.view', 'View per-key API usage analytics'),
    -- Documentation (Section 44)
    ('docs.manage', 'Manage documentation pages'),
    ('docs.admin', 'Manage documentation pages (create, edit, delete, review)'),
    -- Affiliate (Section 55/56)
    ('affiliate.enroll', 'Enroll in affiliate program'),
    ('affiliate.manage', 'Manage affiliate program'),
    ('affiliate.admin.manage', 'Affiliate admin manage'),
    ('affiliate.admin.view', 'Affiliate admin view'),
    ('affiliate.providers.manage', 'Manage affiliate providers'),
    -- AI Context (Section 20)
    ('ai.context.points_balance', 'View AI credit/points balance'),
    ('ai.context.course_progress', 'Access course progress in AI context'),
    ('ai.context.mentorship_activity', 'Access mentorship activity in AI context'),
    ('ai.context.forum_activity', 'Access forum activity in AI context'),
    ('ai.context.referral_activity', 'Access referral activity in AI context'),
    ('ai.context.moderation_history', 'Access moderation history in AI context'),
    ('ai.context.admin_view', 'View AI usage analytics (admin)'),
    ('ai.context.admin_delete_entry', 'Delete AI context entries (admin)'),
    ('ai.generate', 'Generate AI content via the AI gateway'),
    ('ai.hitl.review', 'Review and approve/reject AI-generated output'),
    ('ai.tools.manage', 'Manage AI tool configurations and enablement'),
    -- Admin Core
    ('admin.settings', 'Manage platform settings'),
    ('admin.settings.manage', 'Manage platform settings (alias)'),
    ('admin.users.manage', 'Manage user accounts'),
    ('admin.users.list', 'List users (admin)'),
    ('admin.groups.manage', 'Manage user groups and roles'),
    ('admin.rbac.manage', 'Manage RBAC groups, roles, and permissions'),
    ('admin.spaces.manage', 'Manage all spaces'),
    ('admin.feature_flags.manage', 'Toggle feature flags'),
    ('admin.i18n.manage', 'Manage translations and languages'),
    ('admin.moderation', 'Manage content moderation queue'),
    ('admin.broadcast', 'Send broadcast messages to users'),
    ('admin.impersonate', 'Impersonate other users for support/debugging'),
    -- Moderation (Section 51)
    ('moderation.report', 'Report content or users'),
    ('moderation.warn', 'Issue user warnings'),
    ('moderation.ban', 'Ban users'),
    ('moderation.appeal.review', 'Review moderation appeals'),
    ('moderation.content.review', 'Review flagged content'),
    ('moderation.broadcast', 'Send broadcast notifications'),
    ('reports.review', 'Review and action content reports (admin/moderators)'),
    -- Spam & Safety (Section 41)
    ('spam.ip_block.manage', 'Manage IP blocks'),
    ('spam.censored_words.manage', 'Manage censored word list'),
    ('spam.user_block.manage', 'Block/unblock users'),
    -- Bots (Section 43)
    ('bots.admin', 'Administer known bots and crawler policies'),
    -- GDPR (Section 19)
    ('gdpr.audit_logs.view', 'View audit logs (super_admin)'),
    ('gdpr.consents.view', 'View consent records (admin)'),
    ('gdpr.data_exports.view', 'View data export requests (admin)'),
    ('gdpr.deletions.view', 'View account deletion requests (admin)'),
    -- Geo (Phase 30)
    ('geo.lookup', 'Resolve arbitrary IP addresses via GeoIP'),
    -- Link Preview
    ('link_preview.fetch', 'Fetch link previews from external URLs'),
    -- Feature Flags (Section 42)
    ('features.manage', 'Toggle feature flags for a tenant'),
    -- Feature Check Keys
    ('feature.blog', 'Feature flag: Blog'),
    ('feature.forum', 'Feature flag: Forum'),
    ('feature.dm', 'Feature flag: DMs'),
    ('feature.wallet', 'Feature flag: Wallet'),
    ('feature.ai_chat', 'Feature flag: AI Chat'),
    ('feature.professional', 'Feature flag: Professional profiles'),
    ('feature.ratings_reviews', 'Feature flag: Ratings & Reviews'),
    -- Coupons & Referrals (Section 55/56)
    ('coupons.manage', 'Manage coupons'),
    ('referral.admin', 'Manage referral program settings and view referrals'),
    -- Newsletter (Section 33)
    ('newsletter.admin', 'Manage newsletter subscribers and campaigns'),
    -- Mentorship (Section 35)
    ('mentorship.admin', 'Manage mentorship programs and matches'),
    -- Super-admin Control Panel (Section 21)
    ('control.panel.access', 'Access super-admin control panel'),
    ('control.tenants.manage', 'Manage tenants'),
    ('control.settings.manage', 'Manage system settings'),
    ('control.feature_flags.manage', 'Manage feature flags'),
    ('control.health.view', 'View infrastructure health'),
    ('control.queue.manage', 'Manage job queue'),
    ('control.cache.manage', 'Manage cache'),
    ('control.security.manage', 'Manage security settings'),
    ('control.api_keys.manage', 'Manage API keys'),
    ('control.ai_budget.manage', 'Manage AI budget'),
    ('control.audit_logs.view', 'View audit logs')
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

-- Member role permissions (standard member capabilities — flat taxonomy)
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key IN (
    'content.create', 'content.edit', 'content.reactions.add', 'content.reactions.remove',
    'content.render', 'content.revisions.view', 'content.pin',
    'blog.create', 'blog.edit_own',
    'forum.thread.create', 'forum.thread.edit_own', 'forum.thread.delete_own',
    'forum.reply.create', 'forum.reply.edit_own', 'forum.reply.delete_own',
    'forum.vote', 'forum.poll.vote',
    'spaces.create',
    'moderation.report', 'reports.review',
    'media.upload', 'media.delete_own', 'media.view_own', 'media.library.view_own',
    'media.video.upload',
    'reactions.create', 'reactions.delete.own',
    'bookmarks.create', 'bookmarks.manage',
    'tags.create',
    'follows.create', 'follows.toggle', 'follows.view',
    'notifications.view', 'notifications.manage',
    'dm.send', 'dm.view', 'dm.request', 'dm.manage_privacy',
    'wallet.points.balance', 'wallet.credits.balance', 'wallet.view', 'wallet.convert',
    'wallet.points.earn', 'wallet.credits.spend', 'wallet.points.convert',
    'payments.create', 'payments.view_own',
    'learning.enroll', 'learning.view', 'learning.progress.update',
    'tickets.submit', 'tickets.view_own',
    'organizations.create',
    'professional.create', 'professional_profile.create',
    'professional_profile.edit_own', 'professional_profile.delete_own',
    'analytics.view.own',
    'api.keys.self_manage',
    'reviews.create', 'reviews.edit_own', 'reviews.delete_own',
    'comparison.create', 'comparison.edit_own', 'comparison.delete_own'
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
