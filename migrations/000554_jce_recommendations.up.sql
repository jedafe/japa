-- 000554_jce_recommendations.up.sql
-- JCE Sub-spec 2/3: generated recommendations per comparison/profile.

BEGIN;

CREATE TABLE IF NOT EXISTS comparison_recommendations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    item_id         uuid REFERENCES comparison_items(id) ON DELETE CASCADE,
    profile_id      uuid REFERENCES comparison_profiles(id) ON DELETE SET NULL,
    factor_id       uuid REFERENCES comparison_score_factors(id) ON DELETE SET NULL,
    text            text NOT NULL,
    action_type     text,
    action_url      text,
    priority        int NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comparison_recommendations_tenant
    ON comparison_recommendations(tenant_id);

CREATE INDEX IF NOT EXISTS idx_comparison_recommendations_content
    ON comparison_recommendations(content_id, profile_id, priority DESC);

CREATE INDEX IF NOT EXISTS idx_comparison_recommendations_item
    ON comparison_recommendations(item_id);

-- Recommendation templates for system types.
UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object(
    'recommendation_templates', '[
        {"factor_key": "language_match", "condition": "score_lt", "threshold": 50, "text": "Book a language test to improve your match.", "action_type": "book", "priority": 20},
        {"factor_key": "cost", "condition": "score_lt", "threshold": 40, "text": "Review your budget against this option.", "action_type": "review", "priority": 10},
        {"factor_key": "eligibility", "condition": "status_in", "statuses": ["limited", "ineligible"], "text": "Check the eligibility rules for this option.", "action_type": "check", "priority": 30}
    ]'::jsonb
)
WHERE is_system = true;

COMMIT;
