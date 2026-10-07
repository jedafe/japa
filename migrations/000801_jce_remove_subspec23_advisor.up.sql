-- JCE Sub-specs 2/3 + Sub-spec 4 AI advisor removal (Section 30 removal
-- amendment). Keeps the core comparison engine (Sub-spec 1) and the
-- data-source / freshness / refresh / ingestion-job surface (Sub-spec 4).
--
-- Drops: comparison_profiles, comparison_score_factors, comparison_scores,
-- comparison_recommendations; the smart_mode/score_version metadata columns;
-- the feature.jce.{profiles,smart_compare,scoring,ai_advisor} flags; the
-- jce.{profile,score,factor,recommendation,ai_advisor}.* permission nodes;
-- the advisor/profile/score system settings; and the four seeded advisor
-- AI agent tools.

-- Seeded AI agent tools for the removed advisor surface. The registered
-- handlers no longer exist; leaving these rows would let the executor emit
-- tool calls that always fail.
DELETE FROM public.ai_tools
WHERE name IN ('comparison.analyze', 'comparison.rank', 'comparison.explain', 'comparison.recommend_next_step');

-- Permission nodes for the removed features. role_permissions cascades.
DELETE FROM public.permissions
WHERE key IN (
    'jce.profile.list',
    'jce.profile.view',
    'jce.profile.create',
    'jce.profile.update',
    'jce.profile.delete',
    'jce.score.view',
    'jce.score.recalculate',
    'jce.recommendation.view',
    'jce.factor.manage',
    'jce.ai_advisor.use'
);

-- Feature flags for the removed features.
DELETE FROM public.feature_flags
WHERE key IN ('feature.jce.profiles', 'feature.jce.smart_compare', 'feature.jce.scoring', 'feature.jce.ai_advisor');

-- System settings only consumed by the removed features.
DELETE FROM public.system_settings
WHERE key IN (
    'jce.default_goals',
    'jce.profile_cache_ttl_minutes',
    'jce.score_cache_ttl_minutes',
    'jce.advisor_model',
    'jce.advisor_max_tokens',
    'jce.advisor_cache_ttl_minutes',
    'jce.advisor_recommendation_count',
    'jce.advisor_stream_timeout_seconds'
);

-- Metadata columns only written/read by Smart Compare and the scoring engine.
ALTER TABLE public.comparison_content_metadata
    DROP CONSTRAINT IF EXISTS comparison_content_metadata_smart_mode_check,
    DROP COLUMN IF EXISTS smart_mode,
    DROP COLUMN IF EXISTS score_version;

-- Scoring/profile config keys on comparison_type_definitions are dead after
-- the removal; strip them from existing rows. Retained keys (weights for
-- cost-of-living, scope_chain, sort_options, page_size_options,
-- item_ref_types, max_item_count, fields) are untouched.
UPDATE public.comparison_type_definitions
SET config = config
    - 'smart_modes'
    - 'scoring_enabled'
    - 'allowed_goals'
    - 'goal_factor_map'
    - 'default_weights'
    - 'profile_defaults'
    - 'recommendation_templates'
    - 'eligibility_rules';

-- Removed tables in dependency-safe order (recommendations/scores reference
-- score_factors and profiles; nothing outside this set references them).
DROP TABLE IF EXISTS public.comparison_recommendations;
DROP TABLE IF EXISTS public.comparison_scores;
DROP TABLE IF EXISTS public.comparison_score_factors;
DROP TABLE IF EXISTS public.comparison_profiles;
