-- 000251_readiness_assessment.up.sql
-- Japa Readiness Assessment (Section 71, ADR-071).
-- Adds form_type + scoring_config columns to forms, creates the
-- assessment_results table, and seeds assessment.* settings + RBAC nodes.
--
-- Ref: japaconnect-engineering/sections/71-readiness-assessment.md
--      japaconnect-engineering/adr/ADR-071-japa-readiness-assessment.md

BEGIN;

-- =====================================================================
-- custom_fields: add config JSONB for per-field scoring metadata.
-- Stores option_scores, weight, dimension, and other assessment-specific
-- scoring config. Backward-compatible: defaults to '{}' for non-assessment
-- fields.
-- =====================================================================
ALTER TABLE custom_fields ADD COLUMN IF NOT EXISTS config jsonb NOT NULL DEFAULT '{}';

-- =====================================================================
-- forms: add form_type discriminator + scoring_config JSONB.
-- Backward-compatible: existing forms default to form_type='standard'.
-- =====================================================================
ALTER TABLE forms ADD COLUMN IF NOT EXISTS form_type text NOT NULL DEFAULT 'standard'
    CHECK (form_type IN ('standard', 'assessment'));

ALTER TABLE forms ADD COLUMN IF NOT EXISTS scoring_config jsonb NOT NULL DEFAULT '{}';

-- =====================================================================
-- assessment_results — scored assessment output with dimension breakdown,
-- threshold label, and personalised roadmap.
-- =====================================================================
CREATE TABLE IF NOT EXISTS assessment_results (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id          uuid REFERENCES users(id) ON DELETE CASCADE,
    submission_id    uuid NOT NULL REFERENCES form_submissions(id) ON DELETE CASCADE,
    total_score      int  NOT NULL,
    dimension_scores jsonb NOT NULL DEFAULT '{}',
    threshold_label  text NOT NULL,
    roadmap_content  jsonb NOT NULL DEFAULT '[]',
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_assessment_results_user
    ON assessment_results (tenant_id, user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_assessment_results_tenant
    ON assessment_results (tenant_id, created_at DESC);

-- =====================================================================
-- system_settings — 3 assessment.* keys (Section 71, ADR-071)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'assessment.enabled',           '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'assessment.ai_roadmap_enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'assessment.completion_points',  '"50"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 3 assessment.* nodes (Section 71, ADR-071)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('assessment.take',         'Take the readiness assessment (all users incl. anonymous)'),
    ('assessment.manage',       'Admin: configure assessment form, weights, thresholds'),
    ('assessment.view_results', 'Admin: view all users results and aggregate stats')
ON CONFLICT (key) DO NOTHING;

-- Grant assessment.manage + assessment.view_results to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('assessment.manage', 'assessment.view_results')
ON CONFLICT DO NOTHING;

-- Grant assessment.take to the default 'member' role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'assessment.take'
ON CONFLICT DO NOTHING;

COMMIT;
