-- Final de-tenant migration: remove all tenant_id columns, FKs, indexes, constraints,
-- materialized views, and the tenants table.

BEGIN;

-- Drop materialized views that reference tenant_id first. Their indexes drop with them.
DROP MATERIALIZED VIEW IF EXISTS public.mv_daily_content_stats CASCADE;
DROP MATERIALIZED VIEW IF EXISTS public.mv_daily_space_stats CASCADE;
DROP MATERIALIZED VIEW IF EXISTS public.mv_revenue_streams CASCADE;
DROP MATERIALIZED VIEW IF EXISTS public.mv_top_content_30d CASCADE;
DROP MATERIALIZED VIEW IF EXISTS public.mv_top_content_7d CASCADE;
DROP MATERIALIZED VIEW IF EXISTS public.mv_top_content_90d CASCADE;
DROP MATERIALIZED VIEW IF EXISTS public.mv_user_engagement_stats CASCADE;

-- Drop all tenant_id columns. CASCADE removes dependent FKs, indexes, and constraints.
ALTER TABLE account_deletion_requests DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE achievement_meta DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE activity_events DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE activity_point_rules DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ad_analytics_daily DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ad_campaigns DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ad_events DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ad_networks DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ad_slot_types DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE affiliate_content_commissions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE affiliate_link_events DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE affiliate_program_members DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE affiliate_program_settings DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE affiliate_providers DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ai_agent_sessions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ai_agent_tool_calls DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ai_hitl_reviews DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ai_personas DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ai_providers DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ai_tool_usage DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ai_tools DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ai_usage_log DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE api_keys DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE appointment_alerts DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE arrival_checklist_guides DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE assessment_results DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE attestation_pathways DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE badge_definitions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE blog_series DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE booking_slots DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE booking_slots DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE bookings DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE bookings DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE bookmark_folders DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE bookmarks DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE calendar_entries DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE calendar_saves DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE calendar_subscriptions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE censored_words DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE challenge_definitions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE checklist_progress DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comment_votes DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comments DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE community_organisations DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comparison_content_metadata DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comparison_data_ingestion_jobs DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comparison_data_sources DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comparison_dimensions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comparison_items DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comparison_profiles DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comparison_recommendations DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comparison_score_factors DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comparison_scores DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comparison_type_definitions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE comparison_values DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE consent_records DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE content_affiliate_scan_results DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE content_campaign_items DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE content_campaigns DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE content_embeddings DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE content_featured DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE content_imports DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE content_reports DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE content_reviews DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE content_shares DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE content_thread_links DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE content_type_jsonld_templates DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE content_views DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE contents DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE coupons DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE credential_pathways DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE credit_packs DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE custom_field_entity_types DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE custom_field_groups DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE custom_field_templates DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE custom_fields DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE cv_format_sessions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE data_export_requests DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE device_tokens DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE digest_editions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE discount_codes DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE dm_conversations DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE dm_messages DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE dm_requests DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE doc_pages DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE editor_block_permissions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE email_templates DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE email_verifications DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE enrollments DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE external_data_sources DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE external_data_sources DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE feature_flags DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE feeds DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE follows DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE form_submissions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE forms DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE forum_bookmarks DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE forum_replies DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE forum_tags DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE forum_threads DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE forum_votes DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE housing_guides DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE instructor_payouts DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE instructor_profiles DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE intel_jobs DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE intel_raw_documents DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE intel_records DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE intel_review_log DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE intel_sources DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ip_blocks DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE job_applications DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE known_bots DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE language_score_stories DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE language_test_requirements DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE language_test_resources DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE learning_certificates DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE learning_lessons DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE learning_modules DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE learning_tracks DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE lesson_progress DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE lesson_quizzes DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE media DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE mentor_sessions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE mentor_sessions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE mentorship_connections DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE mentorship_matches DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE mentorship_profiles DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE mentorship_profiles DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE mentorship_programs DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE menu_items DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE menus DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE moderation_alerts DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE monitored_embassies DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE mutual_aid_posts DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE newsletter_campaigns DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE newsletter_lists DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE newsletter_lists DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE newsletter_subscribers DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE notification_batch_windows DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE notifications DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE oauth_accounts DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE onboarding_state DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE onboarding_templates DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE organization_boosts DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE organizations DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ownership_claims DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE page_builder_pages DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE pagebuilder_blocks DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE pagebuilder_templates DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE pagebuilder_themes DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE pages DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE password_history DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE password_reset_tokens DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE payment_webhook_events DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE payments DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE payout_providers DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE persona_voices DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE planning_group_checkins DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE planning_group_members DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE planning_group_milestones DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE planning_groups DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE platform_data_exports DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE platform_data_imports DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE platform_events DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE professional_documents DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE professional_relocations DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE professional_verifiers DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE push_tokens DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE reaction_types DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE reactions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE redirects DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE referral_codes DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE referral_programs DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE referrals DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE remittance_providers DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE remittance_rate_alerts DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE remittance_rates DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE revenue_splits DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE review_aggregates DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE reviews DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE saved_scholarships DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE scam_alert_subscriptions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE scam_reports DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE scholarships DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE search_analytics DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE security_scan_results DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE service_listing_meta DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE share_platforms DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE sidebar_widgets DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE sidebars DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE source_collections DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE space_members DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE space_moderators DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE space_mutes DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE space_permission_presets DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE space_pinned_posts DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE space_types DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE spaces DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE story_meta DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE studio_batches DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE studio_jobs DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE subscription_plan_tracks DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE subscription_plans DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE system_settings DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE tag_moderators DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE tags DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE tax_obligation_guides DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE tax_treaty_pairs DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE template_literals DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ticket_types DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE tickets DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE totp_credentials DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE translations DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE travel_providers DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE trending_snapshots DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE university_course_catalog DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE university_course_offerings DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE university_track_categories DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_affiliate_links DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_badges DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_blocks DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_challenge_progress DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_dm_settings DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_group_memberships DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_group_promotions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_groups DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_moderation_actions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_mutes DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_operation_bans DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_privacy_settings DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_roles DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE user_source_blocks DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE users DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE video_channel_claims DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE video_import_jobs DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE video_sources DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE visa_processing_stats DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE visa_routes DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE visa_timeline_milestones DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE visa_timelines DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE wallet_ledger DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE wallet_settings DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE wallet_withdrawal_fee_tiers DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE wallet_withdrawal_methods DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE wallet_withdrawals DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE webhook_deliveries DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE webhook_endpoints DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE widget_configurations DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE wiki_pages DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE wiki_revisions DROP COLUMN IF EXISTS tenant_id CASCADE;
ALTER TABLE ws_tickets DROP COLUMN IF EXISTS tenant_id CASCADE;

-- Drop the tenants table after all FK references are gone.
DROP TABLE IF EXISTS tenants CASCADE;

-- Add replacement primary keys for tables that lost a PK containing tenant_id.
-- affiliate_program_settings had PRIMARY KEY (tenant_id); no replacement PK added.
ALTER TABLE reactions ADD PRIMARY KEY (entity_type, entity_id, user_id, reaction_key);
ALTER TABLE reaction_types ADD PRIMARY KEY (key);
ALTER TABLE review_aggregates ADD PRIMARY KEY (entity_type, entity_id);
ALTER TABLE user_dm_settings ADD PRIMARY KEY (user_id);

-- Recreate materialized views without tenant_id grouping.
CREATE MATERIALIZED VIEW public.mv_daily_content_stats AS
SELECT (date_trunc('day'::text, platform_events.created_at))::date AS day, platform_events.entity_id AS content_id,
    count(*) FILTER (WHERE (platform_events.event_type = 'content.viewed'::text)) AS views,
    count(*) FILTER (WHERE (platform_events.event_type = 'reaction.given'::text)) AS reactions,
    count(*) FILTER (WHERE (platform_events.event_type = 'comment.created'::text)) AS comments,
    count(DISTINCT platform_events.user_id) AS unique_viewers
   FROM platform_events
  WHERE ((platform_events.event_type = ANY (ARRAY['content.viewed'::text, 'reaction.given'::text, 'comment.created'::text])) AND (platform_events.is_synthetic = false))
  GROUP BY ((date_trunc('day'::text, platform_events.created_at))::date), platform_events.entity_id;
CREATE MATERIALIZED VIEW public.mv_daily_space_stats AS
SELECT (date_trunc('day'::text, platform_events.created_at))::date AS day, platform_events.entity_id AS space_id,
    count(*) FILTER (WHERE (platform_events.event_type = 'space.joined'::text)) AS joins,
    count(*) FILTER (WHERE (platform_events.event_type = 'content.created'::text)) AS posts,
    count(DISTINCT platform_events.user_id) AS active_users
   FROM platform_events
  WHERE ((platform_events.event_type = ANY (ARRAY['space.joined'::text, 'content.created'::text])) AND (platform_events.is_synthetic = false))
  GROUP BY ((date_trunc('day'::text, platform_events.created_at))::date), platform_events.entity_id;
CREATE MATERIALIZED VIEW public.mv_revenue_streams AS
SELECT (date_trunc('month'::text, payments.created_at))::date AS month, sum(payments.amount) FILTER (WHERE (payments.purpose_type = 'pro_subscription'::text)) AS subscription_revenue,
    sum(payments.amount) FILTER (WHERE (payments.purpose_type = 'course_access'::text)) AS course_revenue,
    sum(payments.amount) FILTER (WHERE (payments.purpose_type = 'ad_campaign'::text)) AS ad_revenue,
    sum(payments.amount) FILTER (WHERE (payments.purpose_type = 'credit_pack'::text)) AS credit_revenue
   FROM payments
  WHERE (payments.status = 'completed'::text)
  GROUP BY ((date_trunc('month'::text, payments.created_at))::date);
CREATE MATERIALIZED VIEW public.mv_top_content_30d AS
SELECT mv_daily_content_stats.content_id, sum(mv_daily_content_stats.views) AS total_views,
    sum(mv_daily_content_stats.reactions) AS total_reactions,
    sum(mv_daily_content_stats.comments) AS total_comments,
    ((sum(mv_daily_content_stats.views) + (sum(mv_daily_content_stats.reactions) * (3)::numeric)) + (sum(mv_daily_content_stats.comments) * (5)::numeric)) AS engagement_score
   FROM mv_daily_content_stats
  WHERE (mv_daily_content_stats.day >= (CURRENT_DATE - 30))
  GROUP BY mv_daily_content_stats.content_id
  ORDER BY ((sum(mv_daily_content_stats.views) + (sum(mv_daily_content_stats.reactions) * (3)::numeric)) + (sum(mv_daily_content_stats.comments) * (5)::numeric)) DESC;
CREATE MATERIALIZED VIEW public.mv_top_content_7d AS
SELECT mv_daily_content_stats.content_id, sum(mv_daily_content_stats.views) AS total_views,
    sum(mv_daily_content_stats.reactions) AS total_reactions,
    sum(mv_daily_content_stats.comments) AS total_comments,
    ((sum(mv_daily_content_stats.views) + (sum(mv_daily_content_stats.reactions) * (3)::numeric)) + (sum(mv_daily_content_stats.comments) * (5)::numeric)) AS engagement_score
   FROM mv_daily_content_stats
  WHERE (mv_daily_content_stats.day >= (CURRENT_DATE - 7))
  GROUP BY mv_daily_content_stats.content_id
  ORDER BY ((sum(mv_daily_content_stats.views) + (sum(mv_daily_content_stats.reactions) * (3)::numeric)) + (sum(mv_daily_content_stats.comments) * (5)::numeric)) DESC;
CREATE MATERIALIZED VIEW public.mv_top_content_90d AS
SELECT mv_daily_content_stats.content_id, sum(mv_daily_content_stats.views) AS total_views,
    sum(mv_daily_content_stats.reactions) AS total_reactions,
    sum(mv_daily_content_stats.comments) AS total_comments,
    ((sum(mv_daily_content_stats.views) + (sum(mv_daily_content_stats.reactions) * (3)::numeric)) + (sum(mv_daily_content_stats.comments) * (5)::numeric)) AS engagement_score
   FROM mv_daily_content_stats
  WHERE (mv_daily_content_stats.day >= (CURRENT_DATE - 90))
  GROUP BY mv_daily_content_stats.content_id
  ORDER BY ((sum(mv_daily_content_stats.views) + (sum(mv_daily_content_stats.reactions) * (3)::numeric)) + (sum(mv_daily_content_stats.comments) * (5)::numeric)) DESC;
CREATE MATERIALIZED VIEW public.mv_user_engagement_stats AS
SELECT (date_trunc('day'::text, platform_events.created_at))::date AS day, platform_events.user_id,
    count(*) FILTER (WHERE (platform_events.event_type = 'content.created'::text)) AS posts_created,
    count(*) FILTER (WHERE (platform_events.event_type = 'reaction.given'::text)) AS reactions_given,
    count(*) FILTER (WHERE (platform_events.event_type = 'comment.created'::text)) AS comments_made,
    count(*) FILTER (WHERE (platform_events.event_type = 'content.viewed'::text)) AS views
   FROM platform_events
  WHERE ((platform_events.user_id IS NOT NULL) AND (platform_events.is_synthetic = false))
  GROUP BY ((date_trunc('day'::text, platform_events.created_at))::date), platform_events.user_id;
CREATE UNIQUE INDEX idx_mv_daily_content_stats_unique ON public.mv_daily_content_stats USING btree (day, content_id);
CREATE UNIQUE INDEX idx_mv_daily_space_stats_unique ON public.mv_daily_space_stats USING btree (day, space_id);
CREATE UNIQUE INDEX idx_mv_revenue_streams ON public.mv_revenue_streams USING btree (month);
CREATE UNIQUE INDEX idx_mv_top_content_30d ON public.mv_top_content_30d USING btree ( content_id);
CREATE UNIQUE INDEX idx_mv_top_content_7d ON public.mv_top_content_7d USING btree ( content_id);
CREATE UNIQUE INDEX idx_mv_top_content_90d ON public.mv_top_content_90d USING btree ( content_id);
CREATE UNIQUE INDEX idx_mv_user_engagement_stats_unique ON public.mv_user_engagement_stats USING btree (day, user_id);

COMMIT;
