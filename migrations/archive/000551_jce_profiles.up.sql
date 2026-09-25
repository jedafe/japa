-- 000551_jce_profiles.up.sql
-- JCE Sub-spec 2/3: comparison profiles and goal/type config.

BEGIN;

CREATE TABLE IF NOT EXISTS comparison_profiles (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         uuid REFERENCES users(id) ON DELETE CASCADE,
    title           text NOT NULL,
    source_space_id uuid REFERENCES spaces(id) ON DELETE SET NULL,
    target_space_ids uuid[] NOT NULL DEFAULT '{}',
    goals           jsonb NOT NULL DEFAULT '[]'::jsonb,
    preferences     jsonb NOT NULL DEFAULT '{}'::jsonb,
    demographics    jsonb NOT NULL DEFAULT '{}'::jsonb,
    is_default      boolean NOT NULL DEFAULT false,
    is_template     boolean NOT NULL DEFAULT false,
    deleted_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comparison_profiles_tenant_user
    ON comparison_profiles(tenant_id, user_id)
    WHERE user_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_comparison_profiles_tenant_templates
    ON comparison_profiles(tenant_id, is_template)
    WHERE is_template = true AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_comparison_profiles_one_default
    ON comparison_profiles(tenant_id, user_id)
    WHERE is_default = true AND user_id IS NOT NULL AND deleted_at IS NULL;

-- Update all system comparison types with allowed goals and profile defaults.
UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object(
    'allowed_goals', '[
        {"key": "pr_fast", "label": "Get permanent residency fast", "description": "Prioritise routes with a clear, short PR pathway", "default_weight": 0.25},
        {"key": "low_cost", "label": "Low cost of living", "description": "Minimise tuition, rent, and daily expenses", "default_weight": 0.25},
        {"key": "high_income", "label": "High earning potential", "description": "Maximise post-study or post-move salary", "default_weight": 0.20},
        {"key": "family_friendly", "label": "Family friendly", "description": "Favour schooling, healthcare, and family visa options", "default_weight": 0.15},
        {"key": "scholarships", "label": "Scholarship opportunities", "description": "Favour funding and bursaries", "default_weight": 0.10},
        {"key": "language_ease", "label": "Language ease", "description": "Favour destinations that match my language skills", "default_weight": 0.05}
    ]'::jsonb,
    'profile_defaults', jsonb_build_object(
        'demographics', jsonb_build_object(
            'language_proficiency', 'intermediate',
            'budget_currency', 'USD'
        ),
        'goals', '[]'::jsonb,
        'preferences', '{}'::jsonb
    ),
    'eligibility_rules', '[]'::jsonb,
    'goal_factor_map', jsonb_build_object(
        'pr_fast', '["pr_path", "speed"]'::jsonb,
        'low_cost', '["cost"]'::jsonb,
        'high_income', '["income"]'::jsonb,
        'family_friendly', '["family"]'::jsonb,
        'scholarships', '["funding"]'::jsonb,
        'language_ease', '["language_match"]'::jsonb
    )
)
WHERE is_system = true;

-- Global default goals fallback in system_settings.
INSERT INTO system_settings (tenant_id, key, value)
SELECT '00000000-0000-0000-0000-000000000001', 'jce.default_goals', '[
    {"key": "pr_fast", "label": "Get permanent residency fast", "description": "Prioritise routes with a clear, short PR pathway", "default_weight": 0.25},
    {"key": "low_cost", "label": "Low cost of living", "description": "Minimise tuition, rent, and daily expenses", "default_weight": 0.25},
    {"key": "high_income", "label": "High earning potential", "description": "Maximise post-study or post-move salary", "default_weight": 0.20},
    {"key": "family_friendly", "label": "Family friendly", "description": "Favour schooling, healthcare, and family visa options", "default_weight": 0.15},
    {"key": "scholarships", "label": "Scholarship opportunities", "description": "Favour funding and bursaries", "default_weight": 0.10},
    {"key": "language_ease", "label": "Language ease", "description": "Favour destinations that match my language skills", "default_weight": 0.05}
]'::jsonb
ON CONFLICT (tenant_id, key) DO NOTHING;

INSERT INTO system_settings (tenant_id, key, value)
SELECT '00000000-0000-0000-0000-000000000001', 'jce.profile_cache_ttl_minutes', to_jsonb(5)
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
