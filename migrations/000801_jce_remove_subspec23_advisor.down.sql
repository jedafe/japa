-- Reverts 000801_jce_remove_subspec23_advisor.up.sql — restores the JCE
-- Sub-spec 2/3 + Sub-spec 4 AI advisor schema objects and seed rows.
-- Table contents are NOT restored (they were dropped) — only the
-- platform-owned system score factors and advisor ai_tools rows are
-- re-seeded. Not restored: user/table data, the
-- comparison_type_definitions.config keys stripped by the up migration
-- (smart_modes, scoring_enabled, allowed_goals, goal_factor_map,
-- default_weights, profile_defaults, recommendation_templates,
-- eligibility_rules), and role_permissions grants for the re-seeded
-- permission nodes.

CREATE TABLE IF NOT EXISTS public.comparison_profiles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid,
    title text NOT NULL,
    source_space_id uuid,
    target_space_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    goals jsonb DEFAULT '[]'::jsonb NOT NULL,
    preferences jsonb DEFAULT '{}'::jsonb NOT NULL,
    demographics jsonb DEFAULT '{}'::jsonb NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    is_template boolean DEFAULT false NOT NULL,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.comparison_profiles
    ADD CONSTRAINT comparison_profiles_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.comparison_profiles
    ADD CONSTRAINT comparison_profiles_source_space_id_fkey FOREIGN KEY (source_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.comparison_profiles
    ADD CONSTRAINT comparison_profiles_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE UNIQUE INDEX IF NOT EXISTS idx_comparison_profiles_one_default ON public.comparison_profiles USING btree (user_id) WHERE ((is_default = true) AND (user_id IS NOT NULL) AND (deleted_at IS NULL));
CREATE INDEX IF NOT EXISTS idx_comparison_profiles_templates ON public.comparison_profiles USING btree (is_template) WHERE ((is_template = true) AND (deleted_at IS NULL));
CREATE INDEX IF NOT EXISTS idx_comparison_profiles_user ON public.comparison_profiles USING btree (user_id) WHERE ((user_id IS NOT NULL) AND (deleted_at IS NULL));

CREATE TABLE IF NOT EXISTS public.comparison_score_factors (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    comparison_type_id uuid,
    factor_key text NOT NULL,
    label text NOT NULL,
    description text,
    dimension_slug text,
    data_source text NOT NULL,
    default_weight numeric DEFAULT 0 NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT comparison_score_factors_data_source_check CHECK ((data_source = ANY (ARRAY['dimension'::text, 'resolver_metadata'::text, 'profile'::text, 'eligibility'::text, 'aggregate'::text]))),
    CONSTRAINT comparison_score_factors_default_weight_check CHECK (((default_weight >= (0)::numeric) AND (default_weight <= (1)::numeric)))
);

ALTER TABLE ONLY public.comparison_score_factors
    ADD CONSTRAINT comparison_score_factors_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.comparison_score_factors
    ADD CONSTRAINT comparison_score_factors_comparison_type_id_fkey FOREIGN KEY (comparison_type_id) REFERENCES public.comparison_type_definitions(id) ON DELETE CASCADE;

CREATE UNIQUE INDEX IF NOT EXISTS comparison_score_factors_comparison_type_id_facto_key ON public.comparison_score_factors USING btree (comparison_type_id, factor_key);
CREATE INDEX IF NOT EXISTS idx_comparison_score_factors_system ON public.comparison_score_factors USING btree (is_system) WHERE (is_system = true);
CREATE INDEX IF NOT EXISTS idx_comparison_score_factors_type ON public.comparison_score_factors USING btree (comparison_type_id);

CREATE TABLE IF NOT EXISTS public.comparison_scores (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    item_id uuid NOT NULL,
    factor_id uuid NOT NULL,
    dimension_id uuid,
    profile_id uuid,
    score_value numeric DEFAULT 0 NOT NULL,
    max_value numeric DEFAULT 100 NOT NULL,
    explanation text,
    eligible_status text,
    calculated_at timestamp with time zone DEFAULT now() NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    CONSTRAINT comparison_scores_eligible_status_check CHECK ((eligible_status = ANY (ARRAY['eligible'::text, 'limited'::text, 'ineligible'::text])))
);

ALTER TABLE ONLY public.comparison_scores
    ADD CONSTRAINT comparison_scores_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.comparison_scores
    ADD CONSTRAINT comparison_scores_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.comparison_scores
    ADD CONSTRAINT comparison_scores_dimension_id_fkey FOREIGN KEY (dimension_id) REFERENCES public.comparison_dimensions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.comparison_scores
    ADD CONSTRAINT comparison_scores_factor_id_fkey FOREIGN KEY (factor_id) REFERENCES public.comparison_score_factors(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.comparison_scores
    ADD CONSTRAINT comparison_scores_item_id_fkey FOREIGN KEY (item_id) REFERENCES public.comparison_items(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.comparison_scores
    ADD CONSTRAINT comparison_scores_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES public.comparison_profiles(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS comparison_scores_content_item_factor_profile_key ON public.comparison_scores USING btree (content_id, item_id, factor_id, profile_id);
CREATE INDEX IF NOT EXISTS idx_comparison_scores_calculated ON public.comparison_scores USING btree (content_id, calculated_at);
CREATE INDEX IF NOT EXISTS idx_comparison_scores_content_profile ON public.comparison_scores USING btree (content_id, profile_id, version);
CREATE INDEX IF NOT EXISTS idx_comparison_scores_item ON public.comparison_scores USING btree (item_id);

CREATE TABLE IF NOT EXISTS public.comparison_recommendations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_id uuid NOT NULL,
    item_id uuid,
    profile_id uuid,
    factor_id uuid,
    text text NOT NULL,
    action_type text,
    action_url text,
    priority integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.comparison_recommendations
    ADD CONSTRAINT comparison_recommendations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.comparison_recommendations
    ADD CONSTRAINT comparison_recommendations_content_id_fkey FOREIGN KEY (content_id) REFERENCES public.contents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.comparison_recommendations
    ADD CONSTRAINT comparison_recommendations_factor_id_fkey FOREIGN KEY (factor_id) REFERENCES public.comparison_score_factors(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.comparison_recommendations
    ADD CONSTRAINT comparison_recommendations_item_id_fkey FOREIGN KEY (item_id) REFERENCES public.comparison_items(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.comparison_recommendations
    ADD CONSTRAINT comparison_recommendations_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES public.comparison_profiles(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_comparison_recommendations_content ON public.comparison_recommendations USING btree (content_id, profile_id, priority DESC);
CREATE INDEX IF NOT EXISTS idx_comparison_recommendations_item ON public.comparison_recommendations USING btree (item_id);

ALTER TABLE public.comparison_content_metadata
    ADD COLUMN IF NOT EXISTS smart_mode text DEFAULT 'none'::text,
    ADD COLUMN IF NOT EXISTS score_version integer DEFAULT 1 NOT NULL;

ALTER TABLE ONLY public.comparison_content_metadata
    ADD CONSTRAINT comparison_content_metadata_smart_mode_check CHECK ((smart_mode = ANY (ARRAY['none'::text, 'eligibility'::text, 'cost'::text, 'pathway'::text, 'ai_advisor'::text])));

-- System score factor seed rows (baseline 000735, verbatim; platform-owned
-- reference data, comparison_type_id IS NULL).
INSERT INTO public.comparison_score_factors (id, comparison_type_id, factor_key, label, description, dimension_slug, data_source, default_weight, config, version, is_system, created_at, updated_at) VALUES
('530cac49-e0ba-4707-85ae-416fc56900dc', NULL, 'japa_score', 'JapaScore', 'Overall 0–100 decision score', NULL, 'aggregate', 0, '{"max": 100, "is_output": true}', 1, true, '2026-08-25 18:53:26.197766+01', '2026-08-25 18:53:26.197766+01'),
('b07a841e-ae78-4f79-9c08-9dab3a9f850d', NULL, 'eligibility', 'Eligibility', 'Hard and soft eligibility rules from the comparison type', NULL, 'eligibility', 0.25, '{"penalty_ineligible": 0}', 1, true, '2026-08-25 18:53:26.197766+01', '2026-08-25 18:53:26.197766+01'),
('00a83c67-3af2-4ec0-82aa-d00e23985f20', NULL, 'cost', 'Cost', 'Cost of living, tuition, or fees (lower is better)', NULL, 'resolver_metadata', 0.20, '{"kind": "number", "direction": "lower_is_better", "value_key": "cost", "missing_score": 0}', 1, true, '2026-08-25 18:53:26.197766+01', '2026-08-25 18:53:26.197766+01'),
('787b9086-9b40-49f6-96b0-466bb1395b02', NULL, 'income', 'Income', 'Salary or earning potential (higher is better)', NULL, 'resolver_metadata', 0.15, '{"kind": "number", "direction": "higher_is_better", "value_key": "income", "missing_score": 0}', 1, true, '2026-08-25 18:53:26.197766+01', '2026-08-25 18:53:26.197766+01'),
('78ba349e-103d-4ed9-983f-a5a1c02e7a45', NULL, 'speed', 'Speed', 'Time to outcome (lower is better)', NULL, 'resolver_metadata', 0.10, '{"kind": "number", "direction": "lower_is_better", "value_key": "speed", "missing_score": 0}', 1, true, '2026-08-25 18:53:26.197766+01', '2026-08-25 18:53:26.197766+01'),
('543f80dd-b4f6-4e44-8199-8e56ff7c6e77', NULL, 'language_match', 'Language match', 'How well the item matches the profile language', NULL, 'profile', 0.10, '{"item_key": "language", "profile_key": "demographics.language_proficiency", "match_strategy": "exact"}', 1, true, '2026-08-25 18:53:26.197766+01', '2026-08-25 18:53:26.197766+01'),
('efb75af0-381c-44a4-9ef6-0ec79367bc11', NULL, 'location_match', 'Location match', 'Whether the item is in the profile target spaces', NULL, 'profile', 0.10, '{"item_key": "geo_space_id", "profile_key": "target_space_ids", "match_strategy": "exact"}', 1, true, '2026-08-25 18:53:26.197766+01', '2026-08-25 18:53:26.197766+01'),
('dedfc100-2c4b-4bfb-81c8-3c16a265f6e4', NULL, 'pr_path', 'PR pathway', 'Strength of the permanent residency pathway', NULL, 'resolver_metadata', 0.10, '{"kind": "number", "direction": "higher_is_better", "value_key": "pr_path_ease", "missing_score": 0}', 1, true, '2026-08-25 18:53:26.197766+01', '2026-08-25 18:53:26.197766+01')
ON CONFLICT (comparison_type_id, factor_key) DO NOTHING;

INSERT INTO public.feature_flags (key, enabled, config) VALUES
    ('feature.jce.profiles', true, '{}'),
    ('feature.jce.smart_compare', true, '{}'),
    ('feature.jce.scoring', true, '{}'),
    ('feature.jce.ai_advisor', true, '{}')
ON CONFLICT DO NOTHING;

INSERT INTO public.permissions (id, key, description) VALUES
    ('e773f9a6-9ba1-4e91-ad2f-75513c09b0e8', 'jce.profile.list', 'List comparison profiles'),
    ('13faafe6-2060-42e0-8c0b-a9139e747a9d', 'jce.profile.view', 'View a comparison profile'),
    ('bcf68e8e-5a58-4b61-a20d-f175061c595a', 'jce.profile.create', 'Create a comparison profile'),
    ('32a1a037-0146-4f81-9215-82e4fa8b020f', 'jce.profile.update', 'Update a comparison profile'),
    ('c60c1e28-56ca-429c-8076-9667a44675af', 'jce.profile.delete', 'Delete a comparison profile'),
    ('e52f3715-32e2-4527-832f-6bc4a2ffe71b', 'jce.score.view', 'View JapaScore for a comparison'),
    ('0e85d365-e980-4ea8-9742-2ef6c506d7a0', 'jce.score.recalculate', 'Recalculate JapaScore for a comparison'),
    ('fc374c70-20c8-4805-9019-1ca05404ec61', 'jce.recommendation.view', 'View comparison recommendations'),
    ('0080e874-d5e2-43af-8194-af3915a66abf', 'jce.factor.manage', 'Manage score factors and scoring rules'),
    ('7df35df2-8a4d-4cee-b572-d1495ff8cb04', 'jce.ai_advisor.use', 'Use JCE AI advisor - Ask the AI advisor questions about a comparison')
ON CONFLICT DO NOTHING;

INSERT INTO public.system_settings (id, key, value) VALUES
    ('f8bf486d-98a6-40f9-b22c-b4a7957dc276', 'jce.default_goals', '[{"key": "pr_fast", "label": "Get permanent residency fast", "description": "Prioritise routes with a clear, short PR pathway", "default_weight": 0.25}, {"key": "low_cost", "label": "Low cost of living", "description": "Minimise tuition, rent, and daily expenses", "default_weight": 0.25}, {"key": "high_income", "label": "High earning potential", "description": "Maximise post-study or post-move salary", "default_weight": 0.20}, {"key": "family_friendly", "label": "Family friendly", "description": "Favour schooling, healthcare, and family visa options", "default_weight": 0.15}, {"key": "scholarships", "label": "Scholarship opportunities", "description": "Favour funding and bursaries", "default_weight": 0.10}, {"key": "language_ease", "label": "Language ease", "description": "Favour destinations that match my language skills", "default_weight": 0.05}]'),
    ('6c20e48b-dbd5-4e34-aad8-c4ed79d88db5', 'jce.profile_cache_ttl_minutes', '5'),
    ('4678c271-1ff0-478f-a3b5-5515f3302db9', 'jce.score_cache_ttl_minutes', '15'),
    ('9aae815f-8323-498c-95c7-f02d647376b2', 'jce.advisor_model', '""'),
    ('ffe88e96-94bc-4ffc-a70f-704604bdc523', 'jce.advisor_max_tokens', '1024'),
    ('c7590777-5fff-4275-b141-9e819081ea4d', 'jce.advisor_cache_ttl_minutes', '15')
ON CONFLICT DO NOTHING;

-- Advisor AI agent tool seed rows (baseline 000735, verbatim).
INSERT INTO public.ai_tools (id, name, description, category, config, is_enabled, required_permission, created_at, updated_at) VALUES
('a8a0d479-d3e1-4257-b330-799dc9adddcb', 'comparison.analyze', 'Analyze a comparison against the user profile and return a ranked, explained answer with next actions.', 'agent_comparison', '{"handler": "JCEAnalyze", "is_blocked": false, "parameters": {"type": "object", "required": ["content_id"], "properties": {"question": {"type": "string", "description": "User question, e.g. Which is best for me?"}, "content_id": {"type": "string", "description": "Comparison content UUID"}, "profile_id": {"type": "string", "description": "Optional comparison profile UUID"}}}, "description": "Analyze a comparison and profile, return scores/explanations and next actions.", "function_name": "comparison.analyze", "requires_hitl": false, "reads_user_data": true, "writes_user_data": false}', true, 'jce.ai_advisor.use', '2026-08-31 02:25:17.674821+01', '2026-08-31 02:25:17.674821+01'),
('a4072b6b-4d22-4153-a798-79f4adcd97b8', 'comparison.rank', 'Rank comparison items with reasoning and confidence.', 'agent_comparison', '{"handler": "JCERank", "is_blocked": false, "parameters": {"type": "object", "required": ["content_id"], "properties": {"content_id": {"type": "string"}, "profile_id": {"type": "string"}}}, "description": "Rank items in a comparison with reasoning.", "function_name": "comparison.rank", "requires_hitl": false, "reads_user_data": true, "writes_user_data": false}', true, 'jce.ai_advisor.use', '2026-08-31 02:25:17.674821+01', '2026-08-31 02:25:17.674821+01'),
('7ea5a709-0a9d-4383-83ac-df117869fc12', 'comparison.explain', 'Explain a single JapaScore factor or recommendation for an item.', 'agent_comparison', '{"handler": "JCEExplain", "is_blocked": false, "parameters": {"type": "object", "required": ["content_id", "item_id"], "properties": {"item_id": {"type": "string"}, "content_id": {"type": "string"}, "factor_key": {"type": "string"}}}, "description": "Explain a single score or recommendation.", "function_name": "comparison.explain", "requires_hitl": false, "reads_user_data": true, "writes_user_data": false}', true, 'jce.ai_advisor.use', '2026-08-31 02:25:17.674821+01', '2026-08-31 02:25:17.674821+01'),
('06149c0e-e0cb-4f46-b62c-fcbbb1ba6c70', 'comparison.recommend_next_step', 'Get the top next actions and recommendations for a comparison and profile.', 'agent_comparison', '{"handler": "JCERecommend", "is_blocked": false, "parameters": {"type": "object", "required": ["content_id"], "properties": {"content_id": {"type": "string"}, "profile_id": {"type": "string"}}}, "description": "Get the top next actions and recommendations for a comparison and profile.", "function_name": "comparison.recommend_next_step", "requires_hitl": false, "reads_user_data": true, "writes_user_data": false}', true, 'jce.ai_advisor.use', '2026-08-31 02:25:17.674821+01', '2026-08-31 02:25:17.674821+01')
ON CONFLICT (name) DO NOTHING;
