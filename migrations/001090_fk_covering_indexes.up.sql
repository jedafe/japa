-- 001090 — FK covering indexes for material tables.
-- Audit finding: 288 FK columns had no leftmost index; child-table deletes
-- and join/filter lookups fall back to seq scans, and parent-row deletes
-- take unindexed child scans under the FK check. This migration covers the
-- set on material/hot tables (live rows > 10 or high-traffic domains).
-- Remaining config-scale FKs are intentionally unindexed (write cost > win).

BEGIN;

CREATE INDEX IF NOT EXISTS idx_affiliate_link_events_converted_payment_id ON affiliate_link_events (converted_payment_id);
CREATE INDEX IF NOT EXISTS idx_affiliate_providers_logo_media_id ON affiliate_providers (logo_media_id);
CREATE INDEX IF NOT EXISTS idx_ai_agent_sessions_geo_space_id ON ai_agent_sessions (geo_space_id);
CREATE INDEX IF NOT EXISTS idx_badge_award_events_activity_event_id ON badge_award_events (activity_event_id);
CREATE INDEX IF NOT EXISTS idx_badge_award_events_awarded_by ON badge_award_events (awarded_by);
CREATE INDEX IF NOT EXISTS idx_badge_definitions_icon_media_id ON badge_definitions (icon_media_id);
CREATE INDEX IF NOT EXISTS idx_badge_definitions_season_id ON badge_definitions (season_id);
CREATE INDEX IF NOT EXISTS idx_badge_progress_badge_id ON badge_progress (badge_id);
CREATE INDEX IF NOT EXISTS idx_badge_tiers_badge_id ON badge_tiers (badge_id);
CREATE INDEX IF NOT EXISTS idx_bookmarks_folder_id ON bookmarks (folder_id);
CREATE INDEX IF NOT EXISTS idx_calendar_entries_hitl_reviewed_by ON calendar_entries (hitl_reviewed_by);
CREATE INDEX IF NOT EXISTS idx_calendar_entries_origin_space_id ON calendar_entries (origin_space_id);
CREATE INDEX IF NOT EXISTS idx_calendar_entries_reviewed_by ON calendar_entries (reviewed_by);
CREATE INDEX IF NOT EXISTS idx_calendar_entries_submitted_by ON calendar_entries (submitted_by);
CREATE INDEX IF NOT EXISTS idx_calendar_saves_entry_id ON calendar_saves (entry_id);
CREATE INDEX IF NOT EXISTS idx_calendar_subscriptions_destination_space_id ON calendar_subscriptions (destination_space_id);
CREATE INDEX IF NOT EXISTS idx_comments_removed_by ON comments (removed_by);
CREATE INDEX IF NOT EXISTS idx_comparison_type_definitions_field_group_id ON comparison_type_definitions (field_group_id);
CREATE INDEX IF NOT EXISTS idx_content_views_user_id ON content_views (user_id);
CREATE INDEX IF NOT EXISTS idx_coupon_events_content_id ON coupon_events (content_id);
CREATE INDEX IF NOT EXISTS idx_coupon_events_user_id ON coupon_events (user_id);
CREATE INDEX IF NOT EXISTS idx_coupon_feed_sources_default_provider_id ON coupon_feed_sources (default_provider_id);
CREATE INDEX IF NOT EXISTS idx_coupon_redemptions_listing_meta_id ON coupon_redemptions (listing_meta_id);
CREATE INDEX IF NOT EXISTS idx_coupon_redemptions_service_booking_id ON coupon_redemptions (service_booking_id);
CREATE INDEX IF NOT EXISTS idx_coupon_redemptions_user_id ON coupon_redemptions (user_id);
CREATE INDEX IF NOT EXISTS idx_coupons_created_by ON coupons (created_by);
CREATE INDEX IF NOT EXISTS idx_cv_format_sessions_destination_space_id ON cv_format_sessions (destination_space_id);
CREATE INDEX IF NOT EXISTS idx_cv_format_sessions_hitl_reviewed_by ON cv_format_sessions (hitl_reviewed_by);
CREATE INDEX IF NOT EXISTS idx_cv_format_sessions_origin_space_id ON cv_format_sessions (origin_space_id);
CREATE INDEX IF NOT EXISTS idx_feeds_content_id ON feeds (content_id);
CREATE INDEX IF NOT EXISTS idx_next_action_rules_created_by ON next_action_rules (created_by);
CREATE INDEX IF NOT EXISTS idx_notification_preferences_notification_type ON notification_preferences (notification_type);
CREATE INDEX IF NOT EXISTS idx_oauth_authorization_codes_user_id ON oauth_authorization_codes (user_id);
CREATE INDEX IF NOT EXISTS idx_products_created_by ON products (created_by);
CREATE INDEX IF NOT EXISTS idx_products_geo_space_id ON products (geo_space_id);
CREATE INDEX IF NOT EXISTS idx_products_logo_id ON products (logo_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_permission_id ON role_permissions (permission_id);
CREATE INDEX IF NOT EXISTS idx_spaces_currency_code ON spaces (currency_code);
CREATE INDEX IF NOT EXISTS idx_space_types_field_group_id ON space_types (field_group_id);
CREATE INDEX IF NOT EXISTS idx_ticket_attachments_comment_id ON ticket_attachments (comment_id);
CREATE INDEX IF NOT EXISTS idx_ticket_attachments_media_id ON ticket_attachments (media_id);
CREATE INDEX IF NOT EXISTS idx_ticket_attachments_ticket_id ON ticket_attachments (ticket_id);
CREATE INDEX IF NOT EXISTS idx_ticket_comments_user_id ON ticket_comments (user_id);
CREATE INDEX IF NOT EXISTS idx_ticket_events_user_id ON ticket_events (user_id);
CREATE INDEX IF NOT EXISTS idx_tickets_assigned_group_id ON tickets (assigned_group_id);
CREATE INDEX IF NOT EXISTS idx_ticket_types_auto_assign_group_id ON ticket_types (auto_assign_group_id);
CREATE INDEX IF NOT EXISTS idx_ticket_types_field_group_id ON ticket_types (field_group_id);
CREATE INDEX IF NOT EXISTS idx_user_badges_awarded_by ON user_badges (awarded_by);
CREATE INDEX IF NOT EXISTS idx_user_badges_badge_id ON user_badges (badge_id);
CREATE INDEX IF NOT EXISTS idx_user_content_views_user_id ON user_content_views (user_id);
CREATE INDEX IF NOT EXISTS idx_users_public_group_id ON users (public_group_id);
CREATE INDEX IF NOT EXISTS idx_wallet_ledger_actor_user_id ON wallet_ledger (actor_user_id);
CREATE INDEX IF NOT EXISTS idx_wallet_withdrawals_currency_code ON wallet_withdrawals (currency_code);
CREATE INDEX IF NOT EXISTS idx_wallet_withdrawals_payout_method_id ON wallet_withdrawals (payout_method_id);
CREATE INDEX IF NOT EXISTS idx_wallet_withdrawals_reviewed_by ON wallet_withdrawals (reviewed_by);
CREATE INDEX IF NOT EXISTS idx_wiki_page_feedback_user_id ON wiki_page_feedback (user_id);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_created_by ON wiki_pages (created_by);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_field_group_id ON wiki_pages (field_group_id);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_guide_id ON wiki_pages (guide_id);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_page_template_id ON wiki_pages (page_template_id);
CREATE INDEX IF NOT EXISTS idx_wiki_revisions_hitl_reviewed_by ON wiki_revisions (hitl_reviewed_by);
CREATE INDEX IF NOT EXISTS idx_wiki_revisions_reviewed_by ON wiki_revisions (reviewed_by);
CREATE INDEX IF NOT EXISTS idx_wiki_revisions_submitted_by ON wiki_revisions (submitted_by);
CREATE INDEX IF NOT EXISTS idx_wiki_type_configs_entity_kind ON wiki_type_configs (entity_kind);
CREATE INDEX IF NOT EXISTS idx_wiki_type_page_templates_field_group_id ON wiki_type_page_templates (field_group_id);
COMMIT;
