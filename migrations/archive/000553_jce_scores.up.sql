-- 000553_jce_scores.up.sql
-- JCE Sub-spec 2/3: per-item, per-factor computed scores.

BEGIN;

CREATE TABLE IF NOT EXISTS comparison_scores (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    item_id         uuid NOT NULL REFERENCES comparison_items(id) ON DELETE CASCADE,
    factor_id       uuid NOT NULL REFERENCES comparison_score_factors(id) ON DELETE CASCADE,
    dimension_id    uuid REFERENCES comparison_dimensions(id) ON DELETE SET NULL,
    profile_id      uuid REFERENCES comparison_profiles(id) ON DELETE SET NULL,
    score_value     numeric NOT NULL DEFAULT 0,
    max_value       numeric NOT NULL DEFAULT 100,
    explanation     text,
    eligible_status text CHECK (eligible_status IN ('eligible', 'limited', 'ineligible')),
    calculated_at   timestamptz NOT NULL DEFAULT now(),
    version         int NOT NULL DEFAULT 1,
    UNIQUE(content_id, item_id, factor_id, profile_id)
);

CREATE INDEX IF NOT EXISTS idx_comparison_scores_tenant
    ON comparison_scores(tenant_id);

CREATE INDEX IF NOT EXISTS idx_comparison_scores_content_profile
    ON comparison_scores(content_id, profile_id, version);

CREATE INDEX IF NOT EXISTS idx_comparison_scores_item
    ON comparison_scores(item_id);

CREATE INDEX IF NOT EXISTS idx_comparison_scores_calculated
    ON comparison_scores(content_id, calculated_at);

-- Add smart/scoring config keys to comparison metadata.
UPDATE comparison_content_metadata
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object(
    'profile_id', null,
    'weights_snapshot', '{}'::jsonb,
    'recommendation_summary', '{}'::jsonb,
    'last_scored_at', null
);

COMMIT;
