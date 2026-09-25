BEGIN;

-- 000671_restore_de_tenant_constraints.up.sql
-- Migration 000668 (final de-tenant) dropped tenant_id from ~100 tables with
-- CASCADE, which silently removed every UNIQUE(tenant_id, ...) constraint and
-- every index whose column list contained tenant_id. As a result, dozens of
-- ON CONFLICT upserts fail with SQLSTATE 42P10 ("no unique or exclusion
-- constraint matching the ON CONFLICT specification") — e.g.
-- saved_scholarships (fixed in 000670), newsletter_subscribers(email),
-- checklist_progress, job_applications, device_tokens, push_subscriptions,
-- wallet_withdrawals(idempotency_key), payment_webhook_events(provider,
-- event_id), oauth-style joins, and most per-table slug/content uniques.
-- It also removed hundreds of tenant-prefixed plain indexes.
--
-- This migration restores the de-tenant equivalents: for each dropped
-- UNIQUE(tenant_id, a, b, ...) it recreates UNIQUE(a, b, ...); for each
-- dropped index it recreates the index without tenant_id. Partial-index
-- predicates and NULLS NOT DISTINCT are preserved.
--
-- Before each unique index is created, duplicates that were written while the
-- constraint was absent are removed (the most recently inserted row wins).
-- Dedupe only considers rows where every key column is NOT NULL — a btree
-- unique index never conflicts on NULLs (except NULLS NOT DISTINCT indexes,
-- where the null-inclusive dedupe is used).

-- activity_events: restore unique on (activity_key, reference)
DELETE FROM activity_events a USING activity_events b WHERE a.activity_key IS NOT DISTINCT FROM b.activity_key AND a.reference IS NOT DISTINCT FROM b.reference AND a.activity_key IS NOT NULL AND a.reference IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS activity_events_tenant_id_activity_key_reference_key ON activity_events (activity_key, reference);

-- activity_point_rules: restore unique on (activity_key)
DELETE FROM activity_point_rules a USING activity_point_rules b WHERE a.activity_key IS NOT DISTINCT FROM b.activity_key AND a.activity_key IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS activity_point_rules_tenant_id_activity_key_key ON activity_point_rules (activity_key);

-- ad_analytics_daily: restore unique on (campaign_id, slot_type_id, "date")
DELETE FROM ad_analytics_daily a USING ad_analytics_daily b WHERE a.campaign_id IS NOT DISTINCT FROM b.campaign_id AND a.slot_type_id IS NOT DISTINCT FROM b.slot_type_id AND a."date" IS NOT DISTINCT FROM b."date" AND a.campaign_id IS NOT NULL AND a.slot_type_id IS NOT NULL AND a."date" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS ad_analytics_daily_tenant_id_campaign_id_slot_type_id_date_key ON ad_analytics_daily (campaign_id, slot_type_id, "date");

-- ad_networks: restore unique on (provider)
DELETE FROM ad_networks a USING ad_networks b WHERE a.provider IS NOT DISTINCT FROM b.provider AND a.provider IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS ad_networks_tenant_id_provider_key ON ad_networks (provider);

-- ad_slot_types: restore unique on (slot_name)
DELETE FROM ad_slot_types a USING ad_slot_types b WHERE a.slot_name IS NOT DISTINCT FROM b.slot_name AND a.slot_name IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS ad_slot_types_tenant_id_slot_name_key ON ad_slot_types (slot_name);

-- affiliate_providers: restore unique on (slug)
DELETE FROM affiliate_providers a USING affiliate_providers b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS affiliate_providers_tenant_id_slug_key ON affiliate_providers (slug);

-- ai_tools: restore unique on ("name")
DELETE FROM ai_tools a USING ai_tools b WHERE a."name" IS NOT DISTINCT FROM b."name" AND a."name" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS ai_tools_tenant_id_name_key ON ai_tools ("name");

-- arrival_checklist_guides: restore unique on (destination_space_id, visa_category, applicant_type, "version")
DELETE FROM arrival_checklist_guides a USING arrival_checklist_guides b WHERE a.destination_space_id IS NOT DISTINCT FROM b.destination_space_id AND a.visa_category IS NOT DISTINCT FROM b.visa_category AND a.applicant_type IS NOT DISTINCT FROM b.applicant_type AND a."version" IS NOT DISTINCT FROM b."version" AND a.destination_space_id IS NOT NULL AND a.visa_category IS NOT NULL AND a.applicant_type IS NOT NULL AND a."version" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS arrival_checklist_guides_tenant_id_destination_space_id_vis_key ON arrival_checklist_guides (destination_space_id, visa_category, applicant_type, "version");

-- attestation_pathways: restore unique on (document_type, issuing_space_id, destination_space_id)
DELETE FROM attestation_pathways a USING attestation_pathways b WHERE a.document_type IS NOT DISTINCT FROM b.document_type AND a.issuing_space_id IS NOT DISTINCT FROM b.issuing_space_id AND a.destination_space_id IS NOT DISTINCT FROM b.destination_space_id AND a.document_type IS NOT NULL AND a.issuing_space_id IS NOT NULL AND a.destination_space_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS attestation_pathways_tenant_id_document_type_issuing_space__key ON attestation_pathways (document_type, issuing_space_id, destination_space_id);

-- badge_definitions: restore unique on (slug)
DELETE FROM badge_definitions a USING badge_definitions b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS badge_definitions_tenant_id_slug_key ON badge_definitions (slug);

-- blog_series: restore unique on (slug)
DELETE FROM blog_series a USING blog_series b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS blog_series_tenant_id_slug_key ON blog_series (slug);

-- calendar_subscriptions: restore unique on (user_id, destination_space_id, category)
DELETE FROM calendar_subscriptions a USING calendar_subscriptions b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.destination_space_id IS NOT DISTINCT FROM b.destination_space_id AND a.category IS NOT DISTINCT FROM b.category AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_calendar_subscriptions_unique ON calendar_subscriptions (user_id, destination_space_id, category) NULLS NOT DISTINCT;

-- challenge_definitions: restore unique on (slug)
DELETE FROM challenge_definitions a USING challenge_definitions b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS challenge_definitions_tenant_id_slug_key ON challenge_definitions (slug);

-- checklist_progress: restore unique on (user_id, guide_id, item_key, list_type)
DELETE FROM checklist_progress a USING checklist_progress b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.guide_id IS NOT DISTINCT FROM b.guide_id AND a.item_key IS NOT DISTINCT FROM b.item_key AND a.list_type IS NOT DISTINCT FROM b.list_type AND a.user_id IS NOT NULL AND a.guide_id IS NOT NULL AND a.item_key IS NOT NULL AND a.list_type IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS checklist_progress_tenant_id_user_id_guide_id_item_key_list_typ ON checklist_progress (user_id, guide_id, item_key, list_type);

-- community_organisations: restore unique on (slug)
DELETE FROM community_organisations a USING community_organisations b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS community_organisations_tenant_id_slug_key ON community_organisations (slug);

-- comparison_data_sources: restore unique on (source_name)
DELETE FROM comparison_data_sources a USING comparison_data_sources b WHERE a.source_name IS NOT DISTINCT FROM b.source_name AND a.source_name IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_comparison_data_sources_tenant_name ON comparison_data_sources (source_name);

-- comparison_profiles: restore unique on (user_id)
DELETE FROM comparison_profiles a USING comparison_profiles b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.user_id IS NOT NULL AND (a.is_default = true AND a.user_id IS NOT NULL AND a.deleted_at IS NULL) AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_comparison_profiles_one_default ON comparison_profiles (user_id) WHERE ((is_default = true) AND (user_id IS NOT NULL) AND (deleted_at IS NULL));

-- comparison_score_factors: restore unique on (comparison_type_id, factor_key)
DELETE FROM comparison_score_factors a USING comparison_score_factors b WHERE a.comparison_type_id IS NOT DISTINCT FROM b.comparison_type_id AND a.factor_key IS NOT DISTINCT FROM b.factor_key AND a.comparison_type_id IS NOT NULL AND a.factor_key IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS comparison_score_factors_tenant_id_comparison_type_id_facto_key ON comparison_score_factors (comparison_type_id, factor_key);

-- comparison_scores: restore unique on (content_id, item_id, factor_id, profile_id)
DELETE FROM comparison_scores a USING comparison_scores b WHERE a.content_id IS NOT DISTINCT FROM b.content_id AND a.item_id IS NOT DISTINCT FROM b.item_id AND a.factor_id IS NOT DISTINCT FROM b.factor_id AND a.profile_id IS NOT DISTINCT FROM b.profile_id AND a.content_id IS NOT NULL AND a.item_id IS NOT NULL AND a.factor_id IS NOT NULL AND a.profile_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS comparison_scores_tenant_content_item_factor_profile_key ON comparison_scores (content_id, item_id, factor_id, profile_id);

-- comparison_type_definitions: restore unique on (slug)
DELETE FROM comparison_type_definitions a USING comparison_type_definitions b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS comparison_type_definitions_tenant_id_slug_key ON comparison_type_definitions (slug);

-- content_embeddings: restore unique on (entity_type, entity_id, chunk_index)
DELETE FROM content_embeddings a USING content_embeddings b WHERE a.entity_type IS NOT DISTINCT FROM b.entity_type AND a.entity_id IS NOT DISTINCT FROM b.entity_id AND a.chunk_index IS NOT DISTINCT FROM b.chunk_index AND a.entity_type IS NOT NULL AND a.entity_id IS NOT NULL AND a.chunk_index IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS content_embeddings_tenant_id_entity_type_entity_id_chunk_in_key ON content_embeddings (entity_type, entity_id, chunk_index);

-- content_featured: restore unique on (content_id)
DELETE FROM content_featured a USING content_featured b WHERE a.content_id IS NOT DISTINCT FROM b.content_id AND a.content_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS content_featured_tenant_id_content_id_key ON content_featured (content_id);

-- content_type_jsonld_templates: restore unique on (content_type)
DELETE FROM content_type_jsonld_templates a USING content_type_jsonld_templates b WHERE a.content_type IS NOT DISTINCT FROM b.content_type AND a.content_type IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS content_type_jsonld_templates_tenant_id_content_type_key ON content_type_jsonld_templates (content_type);

-- contents: restore unique on (content_hash)
DELETE FROM contents a USING contents b WHERE a.content_hash IS NOT DISTINCT FROM b.content_hash AND a.content_hash IS NOT NULL AND (a.content_hash IS NOT NULL) AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_contents_tenant_content_hash ON contents (content_hash) WHERE (content_hash IS NOT NULL);

-- contents: restore unique on (slug)
DELETE FROM contents a USING contents b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_contents_tenant_slug ON contents (slug);

-- coupons: restore unique on (code)
DELETE FROM coupons a USING coupons b WHERE a.code IS NOT DISTINCT FROM b.code AND a.code IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS coupons_tenant_id_code_key ON coupons (code);

-- credential_pathways: restore unique on (profession_id, issuing_space_id, destination_space_id)
DELETE FROM credential_pathways a USING credential_pathways b WHERE a.profession_id IS NOT DISTINCT FROM b.profession_id AND a.issuing_space_id IS NOT DISTINCT FROM b.issuing_space_id AND a.destination_space_id IS NOT DISTINCT FROM b.destination_space_id AND a.profession_id IS NOT NULL AND a.issuing_space_id IS NOT NULL AND a.destination_space_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS credential_pathways_tenant_id_profession_id_issuing_space_i_key ON credential_pathways (profession_id, issuing_space_id, destination_space_id);

-- credit_packs: restore unique on ("name")
DELETE FROM credit_packs a USING credit_packs b WHERE a."name" IS NOT DISTINCT FROM b."name" AND a."name" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS credit_packs_tenant_id_name_key ON credit_packs ("name");

-- custom_field_entity_types: restore unique on ("value")
DELETE FROM custom_field_entity_types a USING custom_field_entity_types b WHERE a."value" IS NOT DISTINCT FROM b."value" AND a."value" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS custom_field_entity_types_tenant_id_value_key ON custom_field_entity_types ("value");

-- custom_field_groups: restore unique on (slug)
DELETE FROM custom_field_groups a USING custom_field_groups b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS custom_field_groups_tenant_id_slug_key ON custom_field_groups (slug);

-- custom_field_templates: restore unique on ("name")
DELETE FROM custom_field_templates a USING custom_field_templates b WHERE a."name" IS NOT DISTINCT FROM b."name" AND a."name" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS custom_field_templates_tenant_id_name_key ON custom_field_templates ("name");

-- device_tokens: restore unique on (user_id, device_id)
DELETE FROM device_tokens a USING device_tokens b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.device_id IS NOT DISTINCT FROM b.device_id AND a.user_id IS NOT NULL AND a.device_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS device_tokens_tenant_id_user_id_device_id_key ON device_tokens (user_id, device_id);

-- discount_codes: restore unique on (code)
DELETE FROM discount_codes a USING discount_codes b WHERE a.code IS NOT DISTINCT FROM b.code AND a.code IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS discount_codes_tenant_id_code_key ON discount_codes (code);

-- dm_conversations: restore unique on (participant_1_id, participant_2_id)
DELETE FROM dm_conversations a USING dm_conversations b WHERE a.participant_1_id IS NOT DISTINCT FROM b.participant_1_id AND a.participant_2_id IS NOT DISTINCT FROM b.participant_2_id AND a.participant_1_id IS NOT NULL AND a.participant_2_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS dm_conversations_tenant_id_participant_1_id_participant_2_i_key ON dm_conversations (participant_1_id, participant_2_id);

-- doc_pages: restore unique on (audience, slug)
DELETE FROM doc_pages a USING doc_pages b WHERE a.audience IS NOT DISTINCT FROM b.audience AND a.slug IS NOT DISTINCT FROM b.slug AND a.audience IS NOT NULL AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS doc_pages_tenant_id_audience_slug_key ON doc_pages (audience, slug);

-- editor_block_permissions: restore unique on (block_type, user_group_id)
DELETE FROM editor_block_permissions a USING editor_block_permissions b WHERE a.block_type IS NOT DISTINCT FROM b.block_type AND a.user_group_id IS NOT DISTINCT FROM b.user_group_id AND a.block_type IS NOT NULL AND a.user_group_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS editor_block_permissions_tenant_id_block_type_user_group_id_key ON editor_block_permissions (block_type, user_group_id);

-- email_templates: restore unique on ("type")
DELETE FROM email_templates a USING email_templates b WHERE a."type" IS NOT DISTINCT FROM b."type" AND a."type" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS email_templates_tenant_id_type_key ON email_templates ("type");

-- email_templates: restore unique on (slug)
DELETE FROM email_templates a USING email_templates b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS email_templates_tenant_slug_uniq ON email_templates (slug);

-- forms: restore unique on (slug)
DELETE FROM forms a USING forms b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS forms_tenant_id_slug_key ON forms (slug);

-- forum_tags: restore unique on (slug)
DELETE FROM forum_tags a USING forum_tags b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS forum_tags_tenant_id_slug_key ON forum_tags (slug);

-- forum_threads: restore unique on (slug)
DELETE FROM forum_threads a USING forum_threads b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND (a.deleted_at IS NULL) AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_forum_threads_slug ON forum_threads (slug) WHERE (deleted_at IS NULL);

-- housing_guides: restore unique on (destination_space_id, city_space_id, accommodation_type)
DELETE FROM housing_guides a USING housing_guides b WHERE a.destination_space_id IS NOT DISTINCT FROM b.destination_space_id AND a.city_space_id IS NOT DISTINCT FROM b.city_space_id AND a.accommodation_type IS NOT DISTINCT FROM b.accommodation_type AND a.destination_space_id IS NOT NULL AND a.city_space_id IS NOT NULL AND a.accommodation_type IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS housing_guides_tenant_id_destination_space_id_city_space_id_key ON housing_guides (destination_space_id, city_space_id, accommodation_type);

-- instructor_payouts: restore unique on (instructor_id, period_start, period_end)
DELETE FROM instructor_payouts a USING instructor_payouts b WHERE a.instructor_id IS NOT DISTINCT FROM b.instructor_id AND a.period_start IS NOT DISTINCT FROM b.period_start AND a.period_end IS NOT DISTINCT FROM b.period_end AND a.instructor_id IS NOT NULL AND a.period_start IS NOT NULL AND a.period_end IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS instructor_payouts_tenant_id_instructor_id_period_start_per_key ON instructor_payouts (instructor_id, period_start, period_end);

-- instructor_profiles: restore unique on (user_id)
DELETE FROM instructor_profiles a USING instructor_profiles b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.user_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS instructor_profiles_tenant_id_user_id_key ON instructor_profiles (user_id);

-- intel_jobs: restore unique on (intel_record_id)
DELETE FROM intel_jobs a USING intel_jobs b WHERE a.intel_record_id IS NOT DISTINCT FROM b.intel_record_id AND a.intel_record_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS intel_jobs_tenant_id_intel_record_id_key ON intel_jobs (intel_record_id);

-- intel_records: restore unique on (source_id, external_key)
DELETE FROM intel_records a USING intel_records b WHERE a.source_id IS NOT DISTINCT FROM b.source_id AND a.external_key IS NOT DISTINCT FROM b.external_key AND a.source_id IS NOT NULL AND a.external_key IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS intel_records_tenant_source_external_key ON intel_records (source_id, external_key);

-- intel_sources: restore unique on (slug)
DELETE FROM intel_sources a USING intel_sources b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND (a.slug IS NOT NULL) AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_intel_sources_tenant_slug ON intel_sources (slug) WHERE (slug IS NOT NULL);

-- intel_sources: restore unique on (source_url)
DELETE FROM intel_sources a USING intel_sources b WHERE a.source_url IS NOT DISTINCT FROM b.source_url AND a.source_url IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_intel_sources_tenant_url ON intel_sources (source_url);

-- job_applications: restore unique on (job_id, applicant_id)
DELETE FROM job_applications a USING job_applications b WHERE a.job_id IS NOT DISTINCT FROM b.job_id AND a.applicant_id IS NOT DISTINCT FROM b.applicant_id AND a.job_id IS NOT NULL AND a.applicant_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS job_applications_tenant_id_job_id_applicant_id_key ON job_applications (job_id, applicant_id);

-- language_test_requirements: restore unique on (destination_space_id, visa_category, test_id)
DELETE FROM language_test_requirements a USING language_test_requirements b WHERE a.destination_space_id IS NOT DISTINCT FROM b.destination_space_id AND a.visa_category IS NOT DISTINCT FROM b.visa_category AND a.test_id IS NOT DISTINCT FROM b.test_id AND a.destination_space_id IS NOT NULL AND a.visa_category IS NOT NULL AND a.test_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS language_test_requirements_tenant_id_destination_space_id_v_key ON language_test_requirements (destination_space_id, visa_category, test_id);

-- learning_lessons: restore unique on (slug)
DELETE FROM learning_lessons a USING learning_lessons b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS learning_lessons_tenant_id_slug_key ON learning_lessons (slug);

-- learning_modules: restore unique on (slug)
DELETE FROM learning_modules a USING learning_modules b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS learning_modules_tenant_id_slug_key ON learning_modules (slug);

-- learning_tracks: restore unique on (slug)
DELETE FROM learning_tracks a USING learning_tracks b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS learning_tracks_tenant_id_slug_key ON learning_tracks (slug);

-- menus: restore unique on (slug)
DELETE FROM menus a USING menus b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS menus_tenant_id_slug_key ON menus (slug);

-- monitored_embassies: restore unique on (slug)
DELETE FROM monitored_embassies a USING monitored_embassies b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS monitored_embassies_tenant_id_slug_key ON monitored_embassies (slug);

-- newsletter_subscribers: restore unique on (email)
DELETE FROM newsletter_subscribers a USING newsletter_subscribers b WHERE a.email IS NOT DISTINCT FROM b.email AND a.email IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS newsletter_subscribers_tenant_id_email_key ON newsletter_subscribers (email);

-- onboarding_state: restore unique on (user_id)
DELETE FROM onboarding_state a USING onboarding_state b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.user_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS onboarding_state_tenant_id_user_id_key ON onboarding_state (user_id);

-- onboarding_templates: restore unique on ("name")
DELETE FROM onboarding_templates a USING onboarding_templates b WHERE a."name" IS NOT DISTINCT FROM b."name" AND a."name" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS onboarding_templates_tenant_id_name_key ON onboarding_templates ("name");

-- organizations: restore unique on (slug)
DELETE FROM organizations a USING organizations b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS organizations_tenant_id_slug_key ON organizations (slug);

-- page_builder_pages: restore unique on (slug)
DELETE FROM page_builder_pages a USING page_builder_pages b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS page_builder_pages_tenant_id_slug_key ON page_builder_pages (slug);

-- pagebuilder_blocks: restore unique on (slug)
DELETE FROM pagebuilder_blocks a USING pagebuilder_blocks b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS pagebuilder_blocks_tenant_id_slug_key ON pagebuilder_blocks (slug);

-- pagebuilder_templates: restore unique on (slug)
DELETE FROM pagebuilder_templates a USING pagebuilder_templates b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS pagebuilder_templates_tenant_id_slug_key ON pagebuilder_templates (slug);

-- pagebuilder_themes: restore unique on (slug)
DELETE FROM pagebuilder_themes a USING pagebuilder_themes b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS pagebuilder_themes_tenant_id_slug_key ON pagebuilder_themes (slug);

-- pages: restore unique on (slug)
DELETE FROM pages a USING pages b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS pages_tenant_id_slug_key ON pages (slug);

-- payment_webhook_events: restore unique on (provider, event_id)
DELETE FROM payment_webhook_events a USING payment_webhook_events b WHERE a.provider IS NOT DISTINCT FROM b.provider AND a.event_id IS NOT DISTINCT FROM b.event_id AND a.provider IS NOT NULL AND a.event_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS payment_webhook_events_tenant_id_provider_event_id_key ON payment_webhook_events (provider, event_id);

-- payout_providers: restore unique on ("type")
DELETE FROM payout_providers a USING payout_providers b WHERE a."type" IS NOT DISTINCT FROM b."type" AND a."type" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS payout_providers_tenant_id_type_key ON payout_providers ("type");

-- persona_voices: restore unique on (slug)
DELETE FROM persona_voices a USING persona_voices b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS persona_voices_tenant_id_slug_key ON persona_voices (slug);

-- redirects: restore unique on (source_path)
DELETE FROM redirects a USING redirects b WHERE a.source_path IS NOT DISTINCT FROM b.source_path AND a.source_path IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS redirects_tenant_id_source_path_key ON redirects (source_path);

-- remittance_providers: restore unique on (slug)
DELETE FROM remittance_providers a USING remittance_providers b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS remittance_providers_tenant_id_slug_key ON remittance_providers (slug);

-- remittance_rate_alerts: restore unique on (user_id, send_currency, receive_currency, "direction")
DELETE FROM remittance_rate_alerts a USING remittance_rate_alerts b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.send_currency IS NOT DISTINCT FROM b.send_currency AND a.receive_currency IS NOT DISTINCT FROM b.receive_currency AND a."direction" IS NOT DISTINCT FROM b."direction" AND a.user_id IS NOT NULL AND a.send_currency IS NOT NULL AND a.receive_currency IS NOT NULL AND a."direction" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS remittance_rate_alerts_tenant_id_user_id_send_currency_rece_key ON remittance_rate_alerts (user_id, send_currency, receive_currency, "direction");

-- reviews: restore unique on (entity_type, entity_id, reviewer_user_id)
DELETE FROM reviews a USING reviews b WHERE a.entity_type IS NOT DISTINCT FROM b.entity_type AND a.entity_id IS NOT DISTINCT FROM b.entity_id AND a.reviewer_user_id IS NOT DISTINCT FROM b.reviewer_user_id AND a.entity_type IS NOT NULL AND a.entity_id IS NOT NULL AND a.reviewer_user_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS reviews_tenant_id_entity_type_entity_id_reviewer_user_id_key ON reviews (entity_type, entity_id, reviewer_user_id);

-- scholarships: restore unique on (intel_record_id)
DELETE FROM scholarships a USING scholarships b WHERE a.intel_record_id IS NOT DISTINCT FROM b.intel_record_id AND a.intel_record_id IS NOT NULL AND (a.intel_record_id IS NOT NULL) AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_scholarships_tenant_intel ON scholarships (intel_record_id) WHERE (intel_record_id IS NOT NULL);

-- share_platforms: restore unique on ("key")
DELETE FROM share_platforms a USING share_platforms b WHERE a."key" IS NOT DISTINCT FROM b."key" AND a."key" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS share_platforms_tenant_id_key_key ON share_platforms ("key");

-- sidebars: restore unique on (app, position, slug)
DELETE FROM sidebars a USING sidebars b WHERE a.app IS NOT DISTINCT FROM b.app AND a.position IS NOT DISTINCT FROM b.position AND a.slug IS NOT DISTINCT FROM b.slug AND a.app IS NOT NULL AND a.position IS NOT NULL AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS sidebars_tenant_id_app_position_slug_key ON sidebars (app, position, slug);

-- source_collections: restore unique on (slug)
DELETE FROM source_collections a USING source_collections b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS source_collections_tenant_id_slug_key ON source_collections (slug);

-- space_mutes: restore unique on (user_id, space_id)
DELETE FROM space_mutes a USING space_mutes b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.space_id IS NOT DISTINCT FROM b.space_id AND a.user_id IS NOT NULL AND a.space_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS space_mutes_tenant_id_user_id_space_id_key ON space_mutes (user_id, space_id);

-- story_meta: restore unique on (content_id)
DELETE FROM story_meta a USING story_meta b WHERE a.content_id IS NOT DISTINCT FROM b.content_id AND a.content_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS story_meta_content_tenant_key ON story_meta (content_id);

-- subscription_plans: restore unique on (slug)
DELETE FROM subscription_plans a USING subscription_plans b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_plans_tenant_slug ON subscription_plans (slug);

-- tag_moderators: restore unique on (tag_id, user_id)
DELETE FROM tag_moderators a USING tag_moderators b WHERE a.tag_id IS NOT DISTINCT FROM b.tag_id AND a.user_id IS NOT DISTINCT FROM b.user_id AND a.tag_id IS NOT NULL AND a.user_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS tag_moderators_tenant_id_tag_id_user_id_key ON tag_moderators (tag_id, user_id);

-- tags: restore unique on (slug)
DELETE FROM tags a USING tags b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS tags_tenant_id_slug_key ON tags (slug);

-- tax_obligation_guides: restore unique on (origin_space_id, destination_space_id, employment_status)
DELETE FROM tax_obligation_guides a USING tax_obligation_guides b WHERE a.origin_space_id IS NOT DISTINCT FROM b.origin_space_id AND a.destination_space_id IS NOT DISTINCT FROM b.destination_space_id AND a.employment_status IS NOT DISTINCT FROM b.employment_status AND a.origin_space_id IS NOT NULL AND a.destination_space_id IS NOT NULL AND a.employment_status IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS tax_obligation_guides_tenant_id_origin_space_id_destination_key ON tax_obligation_guides (origin_space_id, destination_space_id, employment_status);

-- tax_treaty_pairs: restore unique on (origin_space_id, destination_space_id)
DELETE FROM tax_treaty_pairs a USING tax_treaty_pairs b WHERE a.origin_space_id IS NOT DISTINCT FROM b.origin_space_id AND a.destination_space_id IS NOT DISTINCT FROM b.destination_space_id AND a.origin_space_id IS NOT NULL AND a.destination_space_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS tax_treaty_pairs_tenant_id_origin_space_id_destination_spac_key ON tax_treaty_pairs (origin_space_id, destination_space_id);

-- template_literals: restore unique on ("key")
DELETE FROM template_literals a USING template_literals b WHERE a."key" IS NOT DISTINCT FROM b."key" AND a."key" IS NOT NULL AND (a.deleted_at IS NULL) AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_template_literals_tenant_key ON template_literals ("key") WHERE (deleted_at IS NULL);

-- ticket_types: restore unique on (slug)
DELETE FROM ticket_types a USING ticket_types b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS ticket_types_tenant_id_slug_key ON ticket_types (slug);

-- totp_credentials: restore unique on (user_id)
DELETE FROM totp_credentials a USING totp_credentials b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.user_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS totp_credentials_tenant_id_user_id_key ON totp_credentials (user_id);

-- translations: restore unique on (locale, "key")
DELETE FROM translations a USING translations b WHERE a.locale IS NOT DISTINCT FROM b.locale AND a."key" IS NOT DISTINCT FROM b."key" AND a.locale IS NOT NULL AND a."key" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS translations_tenant_id_locale_key_key ON translations (locale, "key");

-- travel_providers: restore unique on (slug)
DELETE FROM travel_providers a USING travel_providers b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS travel_providers_tenant_id_slug_key ON travel_providers (slug);


-- university_course_catalog: restore unique on (slug)
DELETE FROM university_course_catalog a USING university_course_catalog b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS university_course_catalog_tenant_id_slug_key ON university_course_catalog (slug);

-- university_course_offerings: restore unique on (course_id, university_id)
DELETE FROM university_course_offerings a USING university_course_offerings b WHERE a.course_id IS NOT DISTINCT FROM b.course_id AND a.university_id IS NOT DISTINCT FROM b.university_id AND a.course_id IS NOT NULL AND a.university_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS university_course_offerings_tenant_id_course_id_university__key ON university_course_offerings (course_id, university_id);

-- university_course_offerings: restore unique on (slug)
DELETE FROM university_course_offerings a USING university_course_offerings b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS university_course_offerings_tenant_id_slug_key ON university_course_offerings (slug);

-- university_track_categories: restore unique on (slug)
DELETE FROM university_track_categories a USING university_track_categories b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS university_track_categories_tenant_id_slug_key ON university_track_categories (slug);

-- user_badges: restore unique on (user_id, badge_id)
DELETE FROM user_badges a USING user_badges b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.badge_id IS NOT DISTINCT FROM b.badge_id AND a.user_id IS NOT NULL AND a.badge_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS user_badges_tenant_id_user_id_badge_id_key ON user_badges (user_id, badge_id);

-- user_challenge_progress: restore unique on (user_id, challenge_id)
DELETE FROM user_challenge_progress a USING user_challenge_progress b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.challenge_id IS NOT DISTINCT FROM b.challenge_id AND a.user_id IS NOT NULL AND a.challenge_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS user_challenge_progress_tenant_id_user_id_challenge_id_key ON user_challenge_progress (user_id, challenge_id);

-- user_group_memberships: restore unique on (user_id, group_id)
DELETE FROM user_group_memberships a USING user_group_memberships b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.group_id IS NOT DISTINCT FROM b.group_id AND a.user_id IS NOT NULL AND a.group_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS user_group_memberships_tenant_id_user_id_group_id_key ON user_group_memberships (user_id, group_id);

-- user_group_promotions: restore unique on (source_group_id, target_group_id)
DELETE FROM user_group_promotions a USING user_group_promotions b WHERE a.source_group_id IS NOT DISTINCT FROM b.source_group_id AND a.target_group_id IS NOT DISTINCT FROM b.target_group_id AND a.source_group_id IS NOT NULL AND a.target_group_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS user_group_promotions_tenant_id_source_group_id_target_grou_key ON user_group_promotions (source_group_id, target_group_id);

-- user_groups: restore unique on (slug)
DELETE FROM user_groups a USING user_groups b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS user_groups_tenant_id_slug_key ON user_groups (slug);

-- user_mutes: restore unique on (user_id, muted_user_id)
DELETE FROM user_mutes a USING user_mutes b WHERE a.user_id IS NOT DISTINCT FROM b.user_id AND a.muted_user_id IS NOT DISTINCT FROM b.muted_user_id AND a.user_id IS NOT NULL AND a.muted_user_id IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS user_mutes_tenant_id_user_id_muted_user_id_key ON user_mutes (user_id, muted_user_id);

-- user_roles: restore unique on (slug)
DELETE FROM user_roles a USING user_roles b WHERE a.slug IS NOT DISTINCT FROM b.slug AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS user_roles_tenant_id_slug_key ON user_roles (slug);

-- visa_processing_stats: restore unique on (origin_space_id, destination_space_id, visa_category, consulate, period_start)
DELETE FROM visa_processing_stats a USING visa_processing_stats b WHERE a.origin_space_id IS NOT DISTINCT FROM b.origin_space_id AND a.destination_space_id IS NOT DISTINCT FROM b.destination_space_id AND a.visa_category IS NOT DISTINCT FROM b.visa_category AND a.consulate IS NOT DISTINCT FROM b.consulate AND a.period_start IS NOT DISTINCT FROM b.period_start AND a.origin_space_id IS NOT NULL AND a.destination_space_id IS NOT NULL AND a.visa_category IS NOT NULL AND a.consulate IS NOT NULL AND a.period_start IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS visa_processing_stats_tenant_id_origin_space_id_destination_key ON visa_processing_stats (origin_space_id, destination_space_id, visa_category, consulate, period_start);

-- visa_routes: restore unique on (destination_space_id, slug)
DELETE FROM visa_routes a USING visa_routes b WHERE a.destination_space_id IS NOT DISTINCT FROM b.destination_space_id AND a.slug IS NOT DISTINCT FROM b.slug AND a.destination_space_id IS NOT NULL AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS visa_routes_tenant_id_destination_space_id_slug_key ON visa_routes (destination_space_id, slug);

-- wallet_withdrawals: restore unique on (idempotency_key)
DELETE FROM wallet_withdrawals a USING wallet_withdrawals b WHERE a.idempotency_key IS NOT DISTINCT FROM b.idempotency_key AND a.idempotency_key IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS wallet_withdrawals_tenant_id_idempotency_key_key ON wallet_withdrawals (idempotency_key);

-- widget_configurations: restore unique on ("name")
DELETE FROM widget_configurations a USING widget_configurations b WHERE a."name" IS NOT DISTINCT FROM b."name" AND a."name" IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_widget_configurations_tenant_name ON widget_configurations ("name");

-- wiki_pages: restore unique on (space_id, slug)
DELETE FROM wiki_pages a USING wiki_pages b WHERE a.space_id IS NOT DISTINCT FROM b.space_id AND a.slug IS NOT DISTINCT FROM b.slug AND a.space_id IS NOT NULL AND a.slug IS NOT NULL AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS wiki_pages_tenant_id_space_id_slug_key ON wiki_pages (space_id, slug);

-- wiki_revisions: restore unique on (wiki_page_id, content_hash)
DELETE FROM wiki_revisions a USING wiki_revisions b WHERE a.wiki_page_id IS NOT DISTINCT FROM b.wiki_page_id AND a.content_hash IS NOT DISTINCT FROM b.content_hash AND a.wiki_page_id IS NOT NULL AND a.content_hash IS NOT NULL AND (a.content_hash IS NOT NULL) AND a.ctid < b.ctid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_revisions_page_hash ON wiki_revisions (wiki_page_id, content_hash) WHERE (content_hash IS NOT NULL);

-- De-tenant plain indexes (tenant_id removed from column list).
CREATE INDEX IF NOT EXISTS idx_achievement_meta_tenant_dest ON achievement_meta (destination_space_id);
CREATE INDEX IF NOT EXISTS idx_achievement_meta_tenant_type ON achievement_meta (milestone_type);
CREATE INDEX IF NOT EXISTS idx_activity_point_rules_key ON activity_point_rules (activity_key);
CREATE INDEX IF NOT EXISTS idx_ad_campaigns_org ON ad_campaigns (organization_id, status) WHERE (organization_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_ad_campaigns_slot ON ad_campaigns (slot_type_id, status);
CREATE INDEX IF NOT EXISTS idx_ad_slot_types_name ON ad_slot_types (slot_name);
CREATE INDEX IF NOT EXISTS idx_affiliate_providers_active ON affiliate_providers (is_active);
CREATE INDEX IF NOT EXISTS idx_ai_agent_sessions_status ON ai_agent_sessions (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_agent_sessions_user ON ai_agent_sessions (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_agent_tool_calls_tool ON ai_agent_tool_calls (tool_name, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_hitl_reviews_source ON ai_hitl_reviews (source, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_hitl_reviews_status ON ai_hitl_reviews (status);
CREATE INDEX IF NOT EXISTS idx_ai_hitl_reviews_target_table ON ai_hitl_reviews (target_table, status);
CREATE INDEX IF NOT EXISTS idx_ai_hitl_reviews_tenant ON ai_hitl_reviews (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_personas_tenant ON ai_personas (status);
CREATE INDEX IF NOT EXISTS idx_ai_providers_tenant_enabled ON ai_providers (enabled);
CREATE INDEX IF NOT EXISTS idx_ai_tool_usage_tenant ON ai_tool_usage (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_tools_tenant ON ai_tools (category);
CREATE INDEX IF NOT EXISTS idx_ai_usage_log_tenant_created ON ai_usage_log (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_log_tenant_provider ON ai_usage_log (provider, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_log_tenant_task ON ai_usage_log (task_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_appointment_alerts_tenant_status ON appointment_alerts (status, embassy_slug);
CREATE INDEX IF NOT EXISTS idx_arrival_checklist_guides_tenant_dest ON arrival_checklist_guides (destination_space_id, is_active);
CREATE INDEX IF NOT EXISTS idx_assessment_results_tenant ON assessment_results (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_assessment_results_user ON assessment_results (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_attestation_pathways_tenant_dest ON attestation_pathways (destination_space_id, is_active);
CREATE INDEX IF NOT EXISTS idx_attestation_pathways_tenant_doc ON attestation_pathways (document_type, is_active);
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant ON audit_logs (action_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_badge_definitions_tenant ON badge_definitions (is_active, display_order);
CREATE INDEX IF NOT EXISTS idx_blog_series_author ON blog_series (author_user_id);
CREATE INDEX IF NOT EXISTS idx_blog_series_slug ON blog_series (slug);
CREATE INDEX IF NOT EXISTS idx_bookmark_folders_user ON bookmark_folders (user_id);
CREATE INDEX IF NOT EXISTS idx_bookmarks_entity ON bookmarks (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_bookmarks_folder ON bookmarks (user_id, folder_id);
CREATE INDEX IF NOT EXISTS idx_bookmarks_user ON bookmarks (user_id);
CREATE INDEX IF NOT EXISTS idx_calendar_entries_tenant_dest_status ON calendar_entries (destination_space_id, status) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_calendar_entries_tenant_occurs_status ON calendar_entries (occurs_at, status) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_challenge_definitions_tenant ON challenge_definitions (is_active, starts_at DESC);
CREATE INDEX IF NOT EXISTS idx_checklist_progress_guide ON checklist_progress (guide_id);
CREATE INDEX IF NOT EXISTS idx_checklist_progress_list_type ON checklist_progress (user_id, list_type);
CREATE INDEX IF NOT EXISTS idx_checklist_progress_user ON checklist_progress (user_id, guide_id);
CREATE INDEX IF NOT EXISTS idx_comment_votes_comment ON comment_votes (comment_id);
CREATE INDEX IF NOT EXISTS idx_comment_votes_user ON comment_votes (user_id);
CREATE INDEX IF NOT EXISTS idx_comments_author ON comments (author_user_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_comments_content ON comments (content_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_comments_parent ON comments (parent_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_community_organisations_intel_record ON community_organisations (intel_record_id) WHERE (intel_record_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_community_orgs_tenant_dest_status ON community_organisations (destination_space_id, status);
CREATE INDEX IF NOT EXISTS idx_community_orgs_tenant_origin ON community_organisations (origin_community, status);
CREATE INDEX IF NOT EXISTS idx_community_orgs_tenant_type ON community_organisations (org_type, status);
CREATE INDEX IF NOT EXISTS idx_comparison_content_metadata_tenant_depth ON comparison_content_metadata (depth);
CREATE INDEX IF NOT EXISTS idx_comp_ingestion_jobs_tenant_content ON comparison_data_ingestion_jobs (content_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_comparison_data_sources_created_by ON comparison_data_sources (created_by_user_id) WHERE (created_by_user_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_comparison_data_sources_entity ON comparison_data_sources (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_comparison_dimensions_slug ON comparison_dimensions (content_id, slug);
CREATE INDEX IF NOT EXISTS idx_comparison_items_intel_record ON comparison_items (intel_record_id) WHERE (intel_record_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_comparison_profiles_tenant_templates ON comparison_profiles (is_template) WHERE ((is_template = true) AND (deleted_at IS NULL));
CREATE INDEX IF NOT EXISTS idx_comparison_profiles_tenant_user ON comparison_profiles (user_id) WHERE ((user_id IS NOT NULL) AND (deleted_at IS NULL));
CREATE INDEX IF NOT EXISTS idx_comparison_score_factors_system ON comparison_score_factors (is_system) WHERE (is_system = true);
CREATE INDEX IF NOT EXISTS idx_comparison_score_factors_tenant_type ON comparison_score_factors (comparison_type_id);
CREATE INDEX IF NOT EXISTS idx_consent_records_tenant ON consent_records (consent_type);
CREATE INDEX IF NOT EXISTS idx_scan_results_unreviewed ON content_affiliate_scan_results (reviewed_at) WHERE (reviewed_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_content_campaigns_tenant_status ON content_campaigns (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_content_embeddings_entity ON content_embeddings (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_content_featured_order ON content_featured (featured_order);
CREATE INDEX IF NOT EXISTS idx_content_imports_tenant ON content_imports (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_content_imports_user ON content_imports (user_id, status);
CREATE INDEX IF NOT EXISTS idx_reports_status ON content_reports (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_content_reviews_content ON content_reviews (content_id);
CREATE INDEX IF NOT EXISTS idx_content_reviews_persona_payload ON content_reviews (source, status, created_at DESC) WHERE (source = 'persona_action'::text);
CREATE INDEX IF NOT EXISTS idx_content_reviews_reviewer ON content_reviews (reviewer_user_id);
CREATE INDEX IF NOT EXISTS idx_content_reviews_source ON content_reviews (source, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_content_reviews_status ON content_reviews (status) WHERE (status = 'pending'::text);
CREATE INDEX IF NOT EXISTS idx_content_shares_content ON content_shares (content_id);
CREATE INDEX IF NOT EXISTS idx_content_shares_user ON content_shares (user_id);
CREATE INDEX IF NOT EXISTS idx_content_thread_links_content ON content_thread_links (content_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_content_thread_links_thread ON content_thread_links (thread_id);
CREATE INDEX IF NOT EXISTS idx_content_views_tenant_content ON content_views (content_id);
CREATE INDEX IF NOT EXISTS idx_content_views_tenant_space ON content_views (space_id);
CREATE INDEX IF NOT EXISTS idx_contents_auto_hidden ON contents (auto_hidden) WHERE (auto_hidden = true);
CREATE INDEX IF NOT EXISTS idx_contents_foryou_window ON contents (status, visibility, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_contents_guide_category ON contents (guide_category) WHERE (guide_category IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_contents_journey ON contents (journey_stage, visa_route) WHERE ((deleted_at IS NULL) AND (status = 'published'::text) AND (visibility = 'public'::text));
CREATE INDEX IF NOT EXISTS idx_contents_org ON contents (organization_id, status, published_at DESC) WHERE ((organization_id IS NOT NULL) AND (deleted_at IS NULL));
CREATE INDEX IF NOT EXISTS idx_contents_soft_delete ON contents (id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_contents_source_target ON contents (source_country, target_country) WHERE ((deleted_at IS NULL) AND (status = 'published'::text) AND (visibility = 'public'::text));
CREATE INDEX IF NOT EXISTS idx_contents_target_country ON contents (target_country) WHERE ((deleted_at IS NULL) AND (status = 'published'::text) AND (visibility = 'public'::text));
CREATE INDEX IF NOT EXISTS idx_coupons_active ON coupons (status, expires_at);
CREATE INDEX IF NOT EXISTS idx_credential_pathways_intel_record ON credential_pathways (intel_record_id) WHERE (intel_record_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_credential_pathways_tenant_dest ON credential_pathways (destination_space_id, is_active);
CREATE INDEX IF NOT EXISTS idx_credential_pathways_tenant_prof ON credential_pathways (profession_id, is_active);
CREATE INDEX IF NOT EXISTS idx_credit_packs_tenant_active ON credit_packs (is_active, "position");
CREATE INDEX IF NOT EXISTS idx_device_tokens_user ON device_tokens (user_id, is_active);
CREATE INDEX IF NOT EXISTS idx_digest_editions_tenant_space ON digest_editions (space_id, published_at DESC) WHERE (status = 'published'::text);
CREATE INDEX IF NOT EXISTS idx_digest_editions_tenant_status ON digest_editions (status, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_doc_pages_audience ON doc_pages (audience);
CREATE INDEX IF NOT EXISTS idx_enrollments_tenant_track ON enrollments (track_id);
CREATE INDEX IF NOT EXISTS idx_enrollments_tenant_user ON enrollments (user_id);
CREATE INDEX IF NOT EXISTS idx_feeds_space ON feeds (space_id, created_at DESC) WHERE (space_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_feeds_user_created ON feeds (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_feeds_user_score ON feeds (user_id, score DESC);
CREATE INDEX IF NOT EXISTS idx_follows_entity ON follows (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_follows_follower ON follows (follower_id, entity_type);
CREATE INDEX IF NOT EXISTS idx_forum_bookmarks_entity ON forum_bookmarks (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_forum_bookmarks_user ON forum_bookmarks (user_id);
CREATE INDEX IF NOT EXISTS idx_forum_replies_ama_pending ON forum_replies (thread_id) WHERE ((ama_pending = true) AND (deleted_at IS NULL));
CREATE INDEX IF NOT EXISTS idx_forum_replies_parent ON forum_replies (parent_reply_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_forum_replies_thread ON forum_replies (thread_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_forum_threads_ama ON forum_threads (created_at DESC) WHERE (ama_config IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_forum_threads_author ON forum_threads (author_user_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_forum_threads_space ON forum_threads (space_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_forum_votes_entity ON forum_votes (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_forum_votes_user ON forum_votes (user_id);
CREATE INDEX IF NOT EXISTS idx_housing_guides_intel_record ON housing_guides (intel_record_id) WHERE (intel_record_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_housing_guides_tenant_dest ON housing_guides (destination_space_id, is_active);
CREATE INDEX IF NOT EXISTS idx_instructor_payouts_instructor ON instructor_payouts (instructor_id, period_start DESC);
CREATE INDEX IF NOT EXISTS idx_instructor_payouts_status ON instructor_payouts (status) WHERE (status = ANY (ARRAY['pending'::text, 'processing'::text]));
CREATE INDEX IF NOT EXISTS idx_instructor_profiles_tenant_status ON instructor_profiles (status);
CREATE INDEX IF NOT EXISTS idx_instructor_profiles_user ON instructor_profiles (user_id);
CREATE INDEX IF NOT EXISTS idx_intel_jobs_source_url ON intel_jobs (source_url);
CREATE INDEX IF NOT EXISTS idx_intel_jobs_tenant_country ON intel_jobs (country);
CREATE INDEX IF NOT EXISTS idx_intel_jobs_tenant_deadline ON intel_jobs (deadline) WHERE (deadline IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_intel_jobs_tenant_job_type ON intel_jobs (job_type) WHERE (job_type IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_intel_jobs_tenant_space ON intel_jobs (country_space_id) WHERE (country_space_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_intel_raw_docs_tenant_hash ON intel_raw_documents (source_id, content_hash);
CREATE INDEX IF NOT EXISTS idx_intel_raw_docs_tenant_source ON intel_raw_documents (source_id, retrieved_at DESC);
CREATE INDEX IF NOT EXISTS idx_intel_records_tenant_entity_status ON intel_records (entity_type, status);
CREATE INDEX IF NOT EXISTS idx_intel_records_tenant_promoted ON intel_records (promoted_record_id) WHERE (promoted_record_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_intel_review_log_tenant_record ON intel_review_log (intel_record_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_intel_sources_category_kind ON intel_sources (category, source_kind) WHERE ((enabled = true) AND (status = 'active'::text));
CREATE INDEX IF NOT EXISTS idx_intel_sources_tenant_category ON intel_sources (category, trust_level);
CREATE INDEX IF NOT EXISTS idx_intel_sources_tenant_enabled_freq ON intel_sources (enabled, crawl_frequency_mins) WHERE enabled;
CREATE INDEX IF NOT EXISTS idx_job_apps_applicant ON job_applications (applicant_id);
CREATE INDEX IF NOT EXISTS idx_job_apps_tenant_status ON job_applications (status);
CREATE INDEX IF NOT EXISTS idx_known_bots_type ON known_bots (bot_type);
CREATE INDEX IF NOT EXISTS idx_language_score_stories_test ON language_score_stories (test_id, status);
CREATE INDEX IF NOT EXISTS idx_language_test_req_tenant_dest ON language_test_requirements (destination_space_id, is_active);
CREATE INDEX IF NOT EXISTS idx_language_test_requirements_intel_record ON language_test_requirements (intel_record_id) WHERE (intel_record_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_language_test_resources_test ON language_test_resources (test_id);
CREATE INDEX IF NOT EXISTS idx_learning_tracks_geo_space ON learning_tracks (geo_space_id) WHERE (geo_space_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_learning_tracks_instructor ON learning_tracks (instructor_id);
CREATE INDEX IF NOT EXISTS idx_learning_tracks_submission_status ON learning_tracks (instructor_id, university_submission_status) WHERE (instructor_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_learning_tracks_university ON learning_tracks (is_published_to_university, category) WHERE (is_published_to_university = true);
CREATE INDEX IF NOT EXISTS idx_lesson_quizzes_tenant_lesson ON lesson_quizzes (lesson_id);
CREATE INDEX IF NOT EXISTS idx_media_quarantined ON media (quarantined_at DESC) WHERE ((quarantine_reason IS NOT NULL) AND (deleted_at IS NULL));
CREATE INDEX IF NOT EXISTS idx_media_tenant ON media (created_at DESC) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_media_type ON media (mime_type, confirmed, deleted_at);
CREATE INDEX IF NOT EXISTS idx_media_uploader ON media (uploader_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_media_user_group ON media (user_group_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_mentorship_conn_tenant_status ON mentorship_connections (status);
CREATE INDEX IF NOT EXISTS idx_mentorship_matches_tenant ON mentorship_matches (status);
CREATE INDEX IF NOT EXISTS idx_mentorship_programs_tenant ON mentorship_programs (is_active);
CREATE INDEX IF NOT EXISTS idx_menu_items_menu ON menu_items (menu_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_moderation_alerts_status ON moderation_alerts (status);
CREATE INDEX IF NOT EXISTS idx_monitored_embassies_tenant_dest ON monitored_embassies (destination_space_id, is_active);
CREATE INDEX IF NOT EXISTS idx_mutual_aid_posts_tenant_expires ON mutual_aid_posts (expires_at) WHERE ((status = 'active'::text) AND (deleted_at IS NULL));
CREATE INDEX IF NOT EXISTS idx_mutual_aid_posts_tenant_status_dest ON mutual_aid_posts (status, destination_space_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_newsletter_campaigns_tenant ON newsletter_campaigns (status);
CREATE INDEX IF NOT EXISTS idx_newsletter_sub_status ON newsletter_subscribers (status);
CREATE INDEX IF NOT EXISTS idx_newsletter_subscribers_tenant ON newsletter_subscribers (status);
CREATE INDEX IF NOT EXISTS idx_notification_batch_windows_tenant ON notification_batch_windows (recipient_id, dispatched, window_close_at);
CREATE INDEX IF NOT EXISTS idx_notifications_recipient ON notifications (recipient_id, read_at, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_type ON notifications (notification_type);
CREATE INDEX IF NOT EXISTS idx_oauth_accounts_provider ON oauth_accounts (provider, provider_user_id);
CREATE INDEX IF NOT EXISTS idx_orgs_tenant ON organizations (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_org_claims_tenant ON ownership_claims (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pagebuilder_templates_active ON pagebuilder_templates (is_active);
CREATE INDEX IF NOT EXISTS idx_pages_page_type ON pages (page_type);
CREATE INDEX IF NOT EXISTS idx_pages_status ON pages (status);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user ON password_reset_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_payout_providers_active ON payout_providers (is_active);
CREATE INDEX IF NOT EXISTS idx_planning_groups_tenant_dest ON planning_groups (destination_space_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_platform_data_exports_tenant ON platform_data_exports (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_platform_data_imports_tenant ON platform_data_imports (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pe_synthetic ON platform_events (event_type, created_at DESC) WHERE (is_synthetic = true);
CREATE INDEX IF NOT EXISTS idx_pe_synthetic_user ON platform_events (user_id, created_at DESC) WHERE (is_synthetic = true);
CREATE INDEX IF NOT EXISTS idx_pe_tenant_type_created ON platform_events (event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_redirects_source ON redirects (source_path) WHERE (is_active = true);
CREATE INDEX IF NOT EXISTS idx_redirects_status ON redirects (is_active);
CREATE INDEX IF NOT EXISTS idx_referral_codes_tenant ON referral_codes (user_id);
CREATE INDEX IF NOT EXISTS idx_referrals_status ON referrals (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_referrals_tenant ON referrals (status);
CREATE INDEX IF NOT EXISTS idx_remittance_providers_tenant ON remittance_providers (is_active, display_order);
CREATE INDEX IF NOT EXISTS idx_rate_alerts_active ON remittance_rate_alerts (send_currency, receive_currency, is_active) WHERE (is_active = true);
CREATE INDEX IF NOT EXISTS idx_remittance_rates_corridor ON remittance_rates (send_currency, receive_currency, provider_id, fetched_at DESC);
CREATE INDEX IF NOT EXISTS idx_remittance_rates_history ON remittance_rates (send_currency, receive_currency, fetched_at DESC);
CREATE INDEX IF NOT EXISTS idx_remittance_rates_intel_record ON remittance_rates (intel_record_id) WHERE (intel_record_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_revenue_splits_instructor_open ON revenue_splits (instructor_id) WHERE (payout_id IS NULL);
CREATE INDEX IF NOT EXISTS idx_revenue_splits_payout ON revenue_splits (payout_id);
CREATE INDEX IF NOT EXISTS idx_review_aggregates_entity ON review_aggregates (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_reviews_entity ON reviews (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_reviews_reviewer ON reviews (reviewer_user_id);
CREATE INDEX IF NOT EXISTS idx_reviews_status ON reviews (status) WHERE (status = 'flagged'::text);
CREATE INDEX IF NOT EXISTS idx_scam_reports_tenant_dest_status ON scam_reports (destination_space_id, status);
CREATE INDEX IF NOT EXISTS idx_scam_reports_tenant_status ON scam_reports (status);
CREATE INDEX IF NOT EXISTS idx_scholarships_tenant_country_deadline ON scholarships (country, deadline);
CREATE INDEX IF NOT EXISTS idx_scholarships_tenant_space ON scholarships (country_space_id);
CREATE INDEX IF NOT EXISTS idx_scholarships_tenant_status_fresh ON scholarships (verification_status, freshness_score);
CREATE INDEX IF NOT EXISTS idx_search_analytics_query ON search_analytics (query, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_search_analytics_zero ON search_analytics (results_count, created_at DESC) WHERE (results_count = 0);
CREATE INDEX IF NOT EXISTS idx_security_scan_tenant ON security_scan_results (severity, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_service_listing_meta_tenant_dest_cat ON service_listing_meta (destination_space_id, service_category);
CREATE INDEX IF NOT EXISTS idx_service_listing_meta_tenant_verified ON service_listing_meta (is_verified) WHERE (is_verified = true);
CREATE INDEX IF NOT EXISTS idx_share_platforms_tenant_enabled ON share_platforms (enabled);
CREATE INDEX IF NOT EXISTS idx_sidebar_widgets_sidebar ON sidebar_widgets (sidebar_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_sidebars_resolve ON sidebars (app, "position", status);
CREATE INDEX IF NOT EXISTS idx_source_collections_tenant ON source_collections (collection_type);
CREATE INDEX IF NOT EXISTS idx_space_mutes_space ON space_mutes (space_id);
CREATE INDEX IF NOT EXISTS idx_space_mutes_user ON space_mutes (user_id);
CREATE INDEX IF NOT EXISTS idx_spaces_active ON spaces (deleted_at) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_spaces_level ON spaces (level);
CREATE INDEX IF NOT EXISTS idx_spaces_name_lower ON spaces (lower(name)) WHERE ((deleted_at IS NULL) AND (visibility = 'public'::text) AND (status = 'active'::text));
CREATE INDEX IF NOT EXISTS idx_story_meta_tenant_dest ON story_meta (destination_space_id);
CREATE INDEX IF NOT EXISTS idx_story_meta_tenant_origin ON story_meta (origin_space_id);
CREATE INDEX IF NOT EXISTS idx_story_meta_tenant_visa ON story_meta (visa_category) WHERE (visa_category IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_story_meta_verified ON story_meta (verified) WHERE (verified = true);
CREATE INDEX IF NOT EXISTS idx_studio_jobs_intel_record ON studio_jobs (intel_record_id) WHERE (intel_record_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_studio_jobs_persona ON studio_jobs (persona_id, status);
CREATE INDEX IF NOT EXISTS idx_studio_jobs_status ON studio_jobs (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_studio_jobs_voice_id ON studio_jobs (voice_id) WHERE (voice_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_subscription_plans_tenant_sort ON subscription_plans (sort_order);
CREATE INDEX IF NOT EXISTS idx_subscription_plans_tenant_status ON subscription_plans (status);
CREATE INDEX IF NOT EXISTS idx_system_settings_is_public ON system_settings (is_public) WHERE (is_public = true);
CREATE INDEX IF NOT EXISTS idx_system_settings_tenant_key ON system_settings (key);
CREATE INDEX IF NOT EXISTS idx_tags_tenant ON tags (follower_count DESC);
CREATE INDEX IF NOT EXISTS idx_tax_obligation_guides_intel_record ON tax_obligation_guides (intel_record_id) WHERE (intel_record_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_tax_obligation_guides_tenant_dest ON tax_obligation_guides (destination_space_id, is_active);
CREATE INDEX IF NOT EXISTS idx_template_literals_tenant_active ON template_literals (is_active) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_translations_tenant_key ON translations (key);
CREATE INDEX IF NOT EXISTS idx_translations_tenant_locale ON translations (locale);
CREATE INDEX IF NOT EXISTS idx_travel_providers_tenant ON travel_providers (is_active);
CREATE INDEX IF NOT EXISTS idx_trending_snapshots_tenant_entity ON trending_snapshots (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_trending_snapshots_trending ON trending_snapshots (entity_type, "window", geo_code, score DESC);
CREATE INDEX IF NOT EXISTS idx_university_course_catalog_tenant_slug ON university_course_catalog (slug) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_university_course_catalog_tenant_title ON university_course_catalog (title) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_university_course_offerings_course ON university_course_offerings (course_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_university_course_offerings_university ON university_course_offerings (university_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_user_badges_user ON user_badges (user_id, awarded_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_blocks_blocker_tenant ON user_blocks (blocker_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_challenge_progress_user ON user_challenge_progress (user_id, completed);
CREATE INDEX IF NOT EXISTS idx_user_group_memberships_group ON user_group_memberships (group_id);
CREATE INDEX IF NOT EXISTS idx_user_group_memberships_user ON user_group_memberships (user_id);
CREATE INDEX IF NOT EXISTS idx_user_moderation_tenant ON user_moderation_actions (action);
CREATE INDEX IF NOT EXISTS idx_user_mutes_muted ON user_mutes (muted_user_id);
CREATE INDEX IF NOT EXISTS idx_user_mutes_user ON user_mutes (user_id);
CREATE INDEX IF NOT EXISTS idx_uob_tenant ON user_operation_bans (operation);
CREATE INDEX IF NOT EXISTS idx_user_privacy_settings_tenant ON user_privacy_settings (user_id);
CREATE INDEX IF NOT EXISTS idx_user_profiles_is_mentor ON user_profiles (is_mentor) WHERE (is_mentor = true);
CREATE INDEX IF NOT EXISTS idx_user_profiles_japa_stage ON user_profiles (japa_stage) WHERE ((japa_stage IS NOT NULL) AND (japa_stage_is_public = true));
CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_is_bot ON users (is_bot) WHERE (is_bot = true);
CREATE INDEX IF NOT EXISTS idx_users_synthetic ON users (is_synthetic) WHERE (is_synthetic = true);
CREATE INDEX IF NOT EXISTS idx_visa_routes_intel_record ON visa_routes (intel_record_id) WHERE (intel_record_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_visa_routes_tenant_dest_active ON visa_routes (destination_space_id, is_active);
CREATE INDEX IF NOT EXISTS idx_visa_routes_tenant_status ON visa_routes (status);
CREATE INDEX IF NOT EXISTS idx_visa_routes_tenant_visa_cat ON visa_routes (visa_category);
CREATE INDEX IF NOT EXISTS idx_wallet_ledger_tenant_user ON wallet_ledger (user_id);
CREATE INDEX IF NOT EXISTS idx_wallet_ledger_tenant_user_source ON wallet_ledger (user_id, source);
CREATE INDEX IF NOT EXISTS idx_wallet_withdrawal_fee_tiers_tenant ON wallet_withdrawal_fee_tiers (is_active, "position");
CREATE INDEX IF NOT EXISTS idx_wallet_withdrawal_methods_default ON wallet_withdrawal_methods (user_id, is_default) WHERE (is_default = true);
CREATE INDEX IF NOT EXISTS idx_wallet_withdrawal_methods_user ON wallet_withdrawal_methods (user_id, is_active);
CREATE INDEX IF NOT EXISTS idx_wallet_withdrawals_status ON wallet_withdrawals (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_wallet_withdrawals_user ON wallet_withdrawals (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_tenant ON webhook_endpoints (status);
CREATE INDEX IF NOT EXISTS idx_widget_configurations_tenant ON widget_configurations (is_active);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_category ON wiki_pages (space_id, category);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_featured ON wiki_pages (is_featured) WHERE (is_featured = true);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_intel_record ON wiki_pages (intel_record_id) WHERE (intel_record_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_tenant_space ON wiki_pages (space_id);
CREATE INDEX IF NOT EXISTS idx_wiki_revisions_tenant_status ON wiki_revisions (status, created_at DESC);
COMMIT;
