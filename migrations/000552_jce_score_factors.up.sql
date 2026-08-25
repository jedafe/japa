-- 000552_jce_score_factors.up.sql
-- JCE Sub-spec 2/3: score factors and per-type scoring config.

BEGIN;

CREATE TABLE IF NOT EXISTS comparison_score_factors (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    comparison_type_id  uuid REFERENCES comparison_type_definitions(id) ON DELETE CASCADE,
    factor_key          text NOT NULL,
    label               text NOT NULL,
    description         text,
    dimension_slug      text,
    data_source         text NOT NULL CHECK (data_source IN ('dimension', 'resolver_metadata', 'profile', 'eligibility', 'aggregate')),
    default_weight      numeric NOT NULL DEFAULT 0 CHECK (default_weight >= 0 AND default_weight <= 1),
    config              jsonb NOT NULL DEFAULT '{}'::jsonb,
    version             int NOT NULL DEFAULT 1,
    is_system           boolean NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, comparison_type_id, factor_key)
);

CREATE INDEX IF NOT EXISTS idx_comparison_score_factors_tenant_type
    ON comparison_score_factors(tenant_id, comparison_type_id);

CREATE INDEX IF NOT EXISTS idx_comparison_score_factors_system
    ON comparison_score_factors(tenant_id, is_system)
    WHERE is_system = true;

-- Add scoring config to system comparison types.
UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object(
    'scoring_enabled', true,
    'default_weights', jsonb_build_object(
        'eligibility', 0.25,
        'cost', 0.20,
        'income', 0.15,
        'speed', 0.10,
        'language_match', 0.10,
        'location_match', 0.10,
        'pr_path', 0.10
    ),
    'smart_modes', jsonb_build_object(
        'eligibility', jsonb_build_object(
            'filter_ineligible', true,
            'factor_multipliers', jsonb_build_object('eligibility', 2.0)
        ),
        'cost', jsonb_build_object(
            'factor_multipliers', jsonb_build_object('cost', 2.0)
        ),
        'pathway', jsonb_build_object(
            'factor_multipliers', jsonb_build_object('pr_path', 1.5, 'speed', 1.5)
        )
    )
)
WHERE is_system = true;

-- Global default score factors. comparison_type_id IS NULL means "global default".
INSERT INTO comparison_score_factors (
    tenant_id, comparison_type_id, factor_key, label, description,
    data_source, default_weight, config, is_system
)
SELECT '00000000-0000-0000-0000-000000000001', NULL, f.*, true
FROM (VALUES
    ('japa_score', 'JapaScore', 'Overall 0–100 decision score', 'aggregate', 0, '{"is_output": true, "max": 100}'::jsonb),
    ('eligibility', 'Eligibility', 'Hard and soft eligibility rules from the comparison type', 'eligibility', 0.25, '{"penalty_ineligible": 0}'::jsonb),
    ('cost', 'Cost', 'Cost of living, tuition, or fees (lower is better)', 'resolver_metadata', 0.20, '{"kind": "number", "direction": "lower_is_better", "value_key": "cost", "missing_score": 0}'::jsonb),
    ('income', 'Income', 'Salary or earning potential (higher is better)', 'resolver_metadata', 0.15, '{"kind": "number", "direction": "higher_is_better", "value_key": "income", "missing_score": 0}'::jsonb),
    ('speed', 'Speed', 'Time to outcome (lower is better)', 'resolver_metadata', 0.10, '{"kind": "number", "direction": "lower_is_better", "value_key": "speed", "missing_score": 0}'::jsonb),
    ('language_match', 'Language match', 'How well the item matches the profile language', 'profile', 0.10, '{"profile_key": "demographics.language_proficiency", "item_key": "language", "match_strategy": "exact"}'::jsonb),
    ('location_match', 'Location match', 'Whether the item is in the profile target spaces', 'profile', 0.10, '{"profile_key": "target_space_ids", "item_key": "geo_space_id", "match_strategy": "exact"}'::jsonb),
    ('pr_path', 'PR pathway', 'Strength of the permanent residency pathway', 'resolver_metadata', 0.10, '{"kind": "number", "direction": "higher_is_better", "value_key": "pr_path_ease", "missing_score": 0}'::jsonb)
) AS f(factor_key, label, description, data_source, default_weight, config)
WHERE NOT EXISTS (
    SELECT 1 FROM comparison_score_factors x
    WHERE x.tenant_id = '00000000-0000-0000-0000-000000000001'
      AND x.comparison_type_id IS NULL
      AND x.factor_key = f.factor_key
);

INSERT INTO system_settings (tenant_id, key, value)
SELECT '00000000-0000-0000-0000-000000000001', 'jce.score_cache_ttl_minutes', to_jsonb(15)
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
