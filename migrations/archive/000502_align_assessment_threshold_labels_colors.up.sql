-- 000502_align_assessment_threshold_labels_colors.up.sql
-- Aligns the default Japa Readiness Assessment scoring_config with ADR-071:
--   1. Threshold labels now match the ADR-071 §1 example labels
--      (Early Stage / Getting Ready / Nearly There / Japa Ready) instead of
--      the divergent seed labels (Not Ready Yet / Getting There / Almost
--      Ready / Japa Ready). BUG-006.
--   2. Threshold colors now store chart theme token names (chart-5, chart-4,
--      chart-2, chart-3) instead of raw hex values, so the palette adapts to
--      light/dark mode via var(--chart-N). BUG-005.
--
-- This updates the single seeded row from migration 000279. The update is
-- idempotent: re-running it produces the same scoring_config.
--
-- Ref: japaconnect-engineering/adr/ADR-071-japa-readiness-assessment.md §1
--      japaconnect-engineering/sections/71-readiness-assessment.md

BEGIN;

UPDATE forms
SET scoring_config = jsonb_build_object(
    'max_score', 100,
    'thresholds', '[
        {"min": 0,  "max": 39,  "label": "Early Stage",   "color": "chart-5"},
        {"min": 40, "max": 69,  "label": "Getting Ready",  "color": "chart-4"},
        {"min": 70, "max": 84,  "label": "Nearly There",   "color": "chart-2"},
        {"min": 85, "max": 100, "label": "Japa Ready",     "color": "chart-3"}
    ]'::jsonb,
    'dimension_weights', jsonb_build_object(
        'financial',    0.35,
        'logistical',   0.30,
        'professional', 0.20,
        'personal',     0.15
    )
)
WHERE id = '00000000-0000-0000-0000-00000000a002'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
