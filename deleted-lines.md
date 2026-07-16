1. 
// Reserved for future features.
func (s *Store) getProfessionalDocumentByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.ProfessionalDocument, error) {
	row, err := s.read.GetProfessionalDocumentByID(ctx, gen.GetProfessionalDocumentByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.ProfessionalDocument{}, fmt.Errorf("getting professional document by ID: %w", err)
	}
	return row, nil
}

2. 
// Reserved for future features.
func (s *Store) getProfessionalRelocationByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.ProfessionalRelocation, error) {
	row, err := s.read.GetProfessionalRelocationByID(ctx, gen.GetProfessionalRelocationByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.ProfessionalRelocation{}, fmt.Errorf("getting professional relocation by ID: %w", err)
	}
	return row, nil
}


3. 
	{Key: "custom_fields.group.view", Description: "View custom field groups (admin listing)"},
		{Key: "custom_fields.group.create", Description: "Create custom field groups"},
		{Key: "custom_fields.group.edit", Description: "Edit custom field groups"},
		{Key: "custom_fields.group.delete", Description: "Delete custom field groups"},
		{Key: "custom_fields.field.create", Description: "Create custom fields"},
		{Key: "custom_fields.field.edit", Description: "Edit custom fields"},
		{Key: "custom_fields.field.delete", Description: "Delete custom fields"},
		{Key: "custom_fields.values.set", Description: "Set custom field values"},
		{Key: "forms.create", Description: "Create forms"},
		{Key: "forms.edit", Description: "Edit forms"},
		{Key: "forms.delete", Description: "Delete forms"},
		{Key: "forms.submissions.view", Description: "View form submissions"},
		{Key: "forms.submissions.edit", Description: "Edit form submission status"},
		{Key: "forms.submissions.delete", Description: "Delete form submissions"},
		{Key: "spaces.create", Description: "Create spaces"},
		{Key: "spaces.edit", Description: "Edit spaces"},
		{Key: "spaces.delete", Description: "Delete spaces"},
		{Key: "spaces.moderate", Description: "Moderate spaces (manage rules and pins)"},
		{Key: "spaces.moderate.any", Description: "Moderate any space (for admin_only posting permission)"},
		{Key: "spaces.moderate.assigned", Description: "Assign or remove space moderators"},
		{Key: "space_types.create", Description: "Create space types"},
		{Key: "space_types.edit", Description: "Edit space types"},
		{Key: "space_types.delete", Description: "Delete space types"},
		{Key: "tags.create", Description: "Create general tags"},
		{Key: "tags.edit", Description: "Edit general tags"},
		{Key: "tags.delete", Description: "Delete general tags"},
		{Key: "spaces.admin", Description: "Manage space permission presets"},
		{Key: "content.create", Description: "Create content"},
		{Key: "content.edit", Description: "Edit content"},
		{Key: "content.delete", Description: "Delete content"},
		{Key: "content.revisions.view", Description: "View content revisions"},
		{Key: "content.pin", Description: "Pin content to spaces"},
		{Key: "content.reaction_types.manage", Description: "Manage reaction types"},
		{Key: "content.reactions.add", Description: "Add reactions to content"},
		{Key: "content.reactions.remove", Description: "Remove reactions from content"},
		{Key: "content.scores.view", Description: "View content scores"},
		{Key: "content.jsonld.manage", Description: "Manage JSON-LD templates"},
		{Key: "content.scheduled.view", Description: "View scheduled content"},
		{Key: "content.page_builder.create", Description: "Create pages with page builder"},
		{Key: "content.page_builder.edit", Description: "Edit pages with page builder"},
		{Key: "content.page_builder.delete", Description: "Delete page builder pages"},
		{Key: "content.render", Description: "Render content via TipTap SSR"},
		{Key: "link_preview.fetch", Description: "Fetch link previews from external URLs"},
		{Key: "follows.toggle", Description: "Follow/unfollow entities"},
		{Key: "follows.view", Description: "View follower/following lists"},
		{Key: "notifications.view", Description: "View notifications"},
		{Key: "notifications.manage", Description: "Manage notification types"},
		{Key: "dm.send", Description: "Send direct messages"},
		{Key: "dm.view", Description: "View direct messages"},
		{Key: "dm.manage_privacy", Description: "Manage DM privacy settings"},
		{Key: "media.upload", Description: "Upload media files"},
		{Key: "media.view_own", Description: "View own uploaded media"},
		{Key: "media.view_all", Description: "View all media (admin)"},
		{Key: "media.delete_own", Description: "Delete own uploaded media"},
		{Key: "media.delete_any", Description: "Delete any media (admin)"},
		{Key: "media.video.upload", Description: "Upload video files"},
		{Key: "media.library.view_own", Description: "View own media library"},
		{Key: "media.library.view_all", Description: "View all media library (admin)"},
		{Key: "media.quarantine.manage", Description: "Review and manage quarantined media (approve/reject)"},
		// Forum Q&A / Solved (Section 08)
		{Key: "forum.thread.create", Description: "Create forum threads"},
		{Key: "forum.thread.edit_own", Description: "Edit own forum threads"},
		{Key: "forum.thread.edit_any", Description: "Edit any forum thread (admin)"},
		{Key: "forum.thread.delete_own", Description: "Delete own forum threads"},
		{Key: "forum.thread.delete_any", Description: "Delete any forum thread (admin)"},
		{Key: "forum.thread.solve", Description: "Mark forum threads as solved"},
		{Key: "forum.reply.create", Description: "Create forum replies"},
		{Key: "forum.reply.edit_own", Description: "Edit own forum replies"},
		{Key: "forum.reply.edit_any", Description: "Edit any forum reply (admin)"},
		{Key: "forum.reply.delete_own", Description: "Delete own forum replies"},
		{Key: "forum.reply.delete_any", Description: "Delete any forum reply (admin)"},
		{Key: "forum.vote", Description: "Vote on forum threads and replies"},
		{Key: "forum.bookmark.manage", Description: "Manage forum bookmarks"},
		{Key: "forum.poll.create", Description: "Create forum polls"},
		{Key: "forum.poll.vote", Description: "Vote on forum polls"},
		{Key: "forum.tag.create", Description: "Create forum tags"},
		{Key: "forum.tag.edit", Description: "Edit forum tags"},
		{Key: "forum.tag.moderate", Description: "Moderate forum tags"},
		{Key: "forum.admin", Description: "Admin-only forum management"},
		// Blog Extension (Section 24)
		{Key: "blog.create", Description: "Create blog posts"},
		{Key: "blog.edit_own", Description: "Edit own blog posts"},
		{Key: "blog.edit_any", Description: "Edit any blog post (admin)"},
		{Key: "blog.publish", Description: "Publish blog posts"},
		{Key: "blog.feature", Description: "Feature/unfeature blog posts"},
		{Key: "blog.series.create", Description: "Create blog series"},
		{Key: "blog.series.edit", Description: "Edit blog series"},
		{Key: "blog.series.delete", Description: "Delete blog series"},
		{Key: "blog.review.create", Description: "Create content reviews"},
		{Key: "blog.review.manage", Description: "Manage content review status"},
		{Key: "blog.rss.view", Description: "View RSS feed"},
		// Universal Ratings & Reviews (Section 29)
		{Key: "reviews.create", Description: "Create reviews"},
		{Key: "reviews.edit_own", Description: "Edit own reviews"},
		{Key: "reviews.edit_any", Description: "Edit any review (admin)"},
		{Key: "reviews.delete_own", Description: "Delete own reviews"},
		{Key: "reviews.delete_any", Description: "Delete any review (admin)"},
		{Key: "reviews.flag", Description: "Flag reviews for moderation"},
		{Key: "reviews.moderate", Description: "Moderate review status"},
		{Key: "reviews.aggregate.view", Description: "View review aggregates"},
		// Generic Comparison System (Section 30)
		{Key: "comparison.type.create", Description: "Create comparison types"},
		{Key: "comparison.type.edit", Description: "Edit comparison types"},
		{Key: "comparison.type.delete", Description: "Delete comparison types"},
		{Key: "comparison.type.access.manage", Description: "Manage comparison type access permissions"},
		{Key: "comparison.create", Description: "Create comparisons"},
		{Key: "comparison.edit_own", Description: "Edit own comparisons"},
		{Key: "comparison.edit_any", Description: "Edit any comparison (admin)"},
		{Key: "comparison.delete_own", Description: "Delete own comparisons"},
		{Key: "comparison.delete_any", Description: "Delete any comparison (admin)"},
		{Key: "comparison.item.manage", Description: "Manage comparison items"},
		{Key: "comparison.dimension.manage", Description: "Manage comparison dimensions"},
		{Key: "comparison.value.manage", Description: "Manage comparison values"},
		// Professional Profiles (Section 33)
		{Key: "professional_profile.type.create", Description: "Create professional profile types"},
		{Key: "professional_profile.type.view", Description: "View professional profile types"},
		{Key: "professional_profile.create", Description: "Create professional profiles"},
		{Key: "professional_profile.edit_own", Description: "Edit own professional profiles"},
		{Key: "professional_profile.edit_any", Description: "Edit any professional profile (admin)"},
		{Key: "professional_profile.delete_own", Description: "Delete own professional profiles"},
		{Key: "professional_profile.delete_any", Description: "Delete any professional profile (admin)"},
		{Key: "professional_profile.member.manage", Description: "Manage professional profile members"},
		{Key: "professional_profile.invite", Description: "Send professional profile invitations"},
		{Key: "professional_profile.author.switch", Description: "Switch authorship to professional profile"},
		// Payment Core (Section 17)
		{Key: "payments.create", Description: "Create payments"},
		{Key: "payments.view_own", Description: "View own payments"},
		{Key: "payments.view_any", Description: "View any payment (admin)"},
		{Key: "payments.refund", Description: "Refund payments"},
		{Key: "payments.webhook", Description: "Handle payment webhooks"},
		{Key: "payments.subscriptions.manage", Description: "Manage subscriptions"},
		{Key: "payments.discount_codes.manage", Description: "Manage discount codes"},
		// Wallet & Credits (Section 28)
		{Key: "wallet.points.balance", Description: "View points balance"},
		{Key: "wallet.credits.balance", Description: "View credits balance"},
		{Key: "wallet.points.earn", Description: "Earn points"},
		{Key: "wallet.credits.spend", Description: "Spend credits"},
		{Key: "wallet.points.convert", Description: "Convert points to credits"},
		{Key: "wallet.credits.purchase", Description: "Purchase credits"},
		{Key: "wallet.admin.adjust", Description: "Admin balance adjustment"},
		{Key: "wallet.admin.rules.manage", Description: "Manage activity point rules"},
		{Key: "wallet.admin.settings.manage", Description: "Manage wallet settings"},
		// Advertising (Section 25)
		{Key: "ad.campaign.manage", Description: "Manage advertising campaigns"},
		{Key: "ad.slot.manage", Description: "Manage advertising slot types"},
		{Key: "ad.stats.view", Description: "View advertising analytics"},
		{Key: "ad.event.track", Description: "Track ad impressions and clicks"},
		// Spam Prevention & Content Safety (Section 41)
		{Key: "spam.ip_block.manage", Description: "Manage IP blocks"},
		{Key: "spam.censored_words.manage", Description: "Manage censored word list"},
		{Key: "spam.user_block.manage", Description: "Block/unblock users"},
		// Ticket System (Section 38)
		{Key: "tickets.submit", Description: "Submit support tickets"},
		{Key: "tickets.view_own", Description: "View own tickets"},
		{Key: "tickets.admin", Description: "Admin ticket management"},
		{Key: "tickets.type.manage", Description: "Manage ticket types"},
		// Content Reports & Flagging (Section 51)
		{Key: "reports.review", Description: "Review and action content reports (admin/moderators)"},
		// GDPR / NDPR Compliance (Section 19)
		{Key: "gdpr.audit_logs.view", Description: "View audit logs (super_admin)"},
		{Key: "gdpr.consents.view", Description: "View consent records (admin)"},
		{Key: "gdpr.data_exports.view", Description: "View data export requests (admin)"},
		{Key: "gdpr.deletions.view", Description: "View account deletion requests (admin)"},
		// Bot & Crawler Management (Section 43)
		{Key: "bots.admin", Description: "Administer known bots and crawler policies"},
		// Platform Analytics & Site Reports (Section 40)
		{Key: "analytics.admin", Description: "View analytics dashboard and security scan results"},
		{Key: "analytics.cache.flush", Description: "Flush platform caches (admin-only)"},
		// Documentation System (Section 44)
		{Key: "docs.admin", Description: "Manage documentation pages (create, edit, delete, review)"},
		// Learning Platform (Section 36)
		{Key: "learning.view", Description: "View published learning tracks and lessons"},
		{Key: "learning.progress.update", Description: "Update lesson progress"},
		{Key: "learning.certificate.issue", Description: "Issue learning certificates"},
		{Key: "learning.admin.manage", Description: "Manage learning tracks, modules, and lessons"},
		// AI Core (Section 20)
		{Key: "ai.generate", Description: "Generate AI content via the AI gateway"},
		{Key: "ai.hitl.review", Description: "Review and approve/reject AI-generated output"},
		{Key: "ai.context.points_balance", Description: "View AI credit/points balance"},
		{Key: "ai.context.admin_view", Description: "View AI usage analytics (admin)"},
		// Platform Settings (admin)
		{Key: "admin.settings", Description: "Manage platform settings"},
		// Admin RBAC Management
		{Key: "admin.rbac.manage", Description: "Manage RBAC groups, roles, and permissions"},
		// Admin Moderation
		{Key: "admin.moderation", Description: "Manage content moderation queue"},
		// Admin Broadcast
		{Key: "admin.broadcast", Description: "Send broadcast messages to users"},
		// Admin Impersonation (Section 46)
		{Key: "admin.impersonate", Description: "Impersonate other users for support/debugging"},
		// Public API & Developer Portal (Section 47)
		{Key: "api.keys.self_manage", Description: "Create, list, and revoke own API keys"},
		{Key: "api.keys.view.all", Description: "View all API keys across all users (admin)"},
		{Key: "api.keys.manage", Description: "Suspend and manage API keys (admin)"},
		{Key: "api.keys.analytics.view", Description: "View API key usage analytics"},
		// i18n Translation System
		{Key: "i18n.manage", Description: "Manage i18n translations (create, update, delete)"},
		// Feature Flags
		{Key: "features.manage", Description: "Toggle feature flags for a tenant"},
		// AI Tool Registry (Section 22)
		{Key: "ai.tools.manage", Description: "Manage AI tool configurations and enablement"},
		// Webhook Admin CRUD (Section 23 — Integrations)
		{Key: "integrations.webhooks.manage", Description: "Manage webhook endpoint configurations"},
		// Newsletter (Section 33)
		{Key: "newsletter.admin", Description: "Manage newsletter subscribers and campaigns"},
		// Referral Program (Section 34)
		{Key: "referral.admin", Description: "Manage referral program settings and view referrals"},
		// Mentorship (Section 35)
		{Key: "mentorship.admin", Description: "Manage mentorship programs and matches"},
		// Page Builder (Section 16)
		{Key: "pagebuilder.blocks.manage", Description: "Manage page builder blocks (create, edit, delete)"},
		{Key: "pagebuilder.templates.manage", Description: "Manage page builder templates (create, edit, delete)"},
		{Key: "pagebuilder.themes.manage", Description: "Manage page builder themes (create, edit, delete, set default)"},
		{Key: "pages.manage", Description: "Manage pages (create, edit, delete)"},
		{Key: "pages.publish", Description: "Publish and archive pages"},
		// Geo IP (Phase 30)
		{Key: "geo.lookup", Description: "Resolve arbitrary IP addresses via GeoIP"},
		// Super-admin Control Panel (Section 21)
		{Key: "control.panel.access", Description: "Access super-admin control panel"},
		{Key: "control.tenants.manage", Description: "Manage tenants"},
		{Key: "control.settings.manage", Description: "Manage system settings"},
		{Key: "control.feature_flags.manage", Description: "Manage feature flags"},
		{Key: "control.health.view", Description: "View infrastructure health"},
		{Key: "control.queue.manage", Description: "Manage job queue"},
		{Key: "control.cache.manage", Description: "Manage cache"},
		{Key: "control.security.manage", Description: "Manage security settings"},
		{Key: "control.api_keys.manage", Description: "Manage API keys"},
		{Key: "control.ai_budget.manage", Description: "Manage AI budget"},
		{Key: "control.audit_logs.view", Description: "View audit logs"},
		// Admin Users & Analytics (previously missing from bootstrap)
		{Key: "admin.users.list", Description: "List platform users"},
		{Key: "admin.users.manage", Description: "View user sessions and activity"},
		{Key: "admin.analytics.search", Description: "Search analytics data"},
		// User Management
		{Key: "user.delete", Description: "Delete user accounts"},
		// Affiliate System
		{Key: "affiliate.admin.view", Description: "View affiliate admin dashboard"},
		{Key: "affiliate.admin.manage", Description: "Manage affiliate admin settings"},
		{Key: "affiliate.providers.manage", Description: "Manage affiliate providers"},
		// Coupons
		{Key: "coupons.manage", Description: "Manage discount coupons"},
		// Moderation Alerts
		{Key: "moderation.alerts.manage", Description: "View and acknowledge moderation alerts"},
		// Space Moderators
		{Key: "spaces.moderator.assign", Description: "Assign moderators to spaces"},
		// AI Context
		{Key: "ai.context.admin_delete_entry", Description: "Delete AI context entries"},
	}
	var defaultTenantID pgtype.UUID
	if err := defaultTenantID.Scan("00000000-0000-0000-0000-000000000001"); err != nil {
		slog.Error("bootstrapping permissions: scanning default tenant ID", "error", err)
		return
	}
	// Register every permission key (idempotent).
	for _, perm := range permissions {
		if err := rbacStore.RegisterPermission(ctx, perm.Key, perm.Description); err != nil {
			slog.Error("bootstrapping permissions: registering permission", "key", perm.Key, "error", err)
		}
	}
	// Create the super_admin_role for the default tenant (idempotent).
	_, err := database.Writer.Exec(ctx, `
		INSERT INTO user_roles (tenant_id, name, slug)
		VALUES ($1, 'Super Admin Role', 'super_admin_role')
		ON CONFLICT (tenant_id, slug) DO NOTHING
	`, defaultTenantID)
	if err != nil {
		slog.Error("bootstrapping permissions: creating super_admin role", "error", err)
		return
	}
	// Resolve the role ID (the INSERT above may have been a no-op).
	var roleID pgtype.UUID
	err = database.Reader.QueryRow(ctx, `
		SELECT id FROM user_roles WHERE tenant_id = $1 AND slug = 'super_admin_role'
	`, defaultTenantID).Scan(&roleID)
	if err != nil {
		slog.Error("bootstrapping permissions: resolving super_admin role ID", "error", err)
		return
	}
	// Resolve the super_admin system group ID by slug.
	var groupID pgtype.UUID
	err = database.Reader.QueryRow(ctx, `
		SELECT id FROM user_groups WHERE tenant_id = $1 AND slug = 'super_admin'
	`, defaultTenantID).Scan(&groupID)
	if err != nil {
		slog.Error("bootstrapping permissions: resolving super_admin group ID", "error", err)
		return
	}
	// Assign every permission to the super_admin role.
	for _, perm := range permissions {
		if err := rbacStore.AssignPermissionToRole(ctx, roleID, perm.Key); err != nil {
			slog.Error("bootstrapping permissions: assigning permission to role", "key", perm.Key, "error", err)
		}
	}
	// Grant the super_admin role to the super_admin system group.
	if err := rbacStore.AssignRoleToGroup(ctx, groupID, roleID); err != nil {
		slog.Error("bootstrapping permissions: assigning role to group", "error", err)
		return
	}
	slog.Info("permissions bootstrapped", "permissions", len(permissions), "role", "super_admin_role")
}

