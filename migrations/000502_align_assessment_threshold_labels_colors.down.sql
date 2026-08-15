-- 000502_align_assessment_threshold_labels_colors.down.sql
-- Reverses 000502: restores the original seed scoring_config from migration
-- 000279 (divergent labels + raw hex colors).

BEGIN;

UPDATE forms
SET scoring_config = '{
    "max_score": 100,
    "thresholds": [
        {"min": 0,  "max": 39,  "label": "Not Ready Yet",   "color": "#ef4444"},
        {"min": 40, "max": 59,  "label": "Getting There",   "color": "#f59e0b"},
        {"min": 60, "max": 74,  "label": "Almost Ready",    "color": "#3b82f6"},
        {"min": 75, "max": 100, "label": "Japa Ready",      "color": "#22c55e"}
    ],
    "dimension_weights": {
        "financial":    0.35,
        "logistical":   0.30,
        "professional": 0.20,
        "personal":     0.15
    }
}'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000a002'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
