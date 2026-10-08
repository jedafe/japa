-- 000807_unified_rollout.up.sql
-- Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §3.3, §7

-- 1) Roadmap status gains 'testing' (§113 lifecycle extension).
ALTER TABLE roadmap_items DROP CONSTRAINT IF EXISTS roadmap_items_status_check;
ALTER TABLE roadmap_items ADD CONSTRAINT roadmap_items_status_check
  CHECK (status IN ('pending','under_review','planned','in_progress',
                    'testing','shipped','declined','archived'));

-- 2) Backfill feature_flags from system_settings bool keys so services
--    migrating off GetBoolOr("X.enabled") keep the operator's choice.
--    EXPLICIT key list — do NOT use a regex: keys like
--    'gamification.badges_enabled', 'ai.cache_enabled',
--    'roadmap.submissions_enabled', 'assessment.ai_roadmap_enabled' are
--    settings (quotas/switches), not feature-reachability toggles, and a
--    pattern match would mint dozens of bogus flags.
--    The list below is the same-name subset of the Task-13 migration map;
--    extend it if Task 13 uncovers more reachability keys.
--    value is jsonb; `value #>> '{}'` unwraps '"true"' and true alike.
INSERT INTO feature_flags (key, enabled, updated_at)
SELECT s.key, COALESCE((s.value #>> '{}')::boolean, true), now()
FROM system_settings s
WHERE s.key = ANY(ARRAY[
  'travel.enabled','story.enabled','wiki.enabled','checklist.enabled',
  'assessment.enabled','remittance.enabled','jobs.enabled','fx.enabled',
  'visa_routes.enabled','scam_alert.enabled','planning_group.enabled',
  'digest.enabled','mutual_aid.enabled','calendar.enabled',
  'japa_stage.enabled','achievement.enabled','service_directory.enabled',
  'scholarship_finder.enabled','arrival_checklist.enabled',
  'appointment_alert.enabled','attestation_guide.enabled',
  'campaign.enabled','col_calculator.enabled','community_locator.enabled',
  'cv_formatter.enabled','housing_guide.enabled','intel.enabled',
  'language_test_hub.enabled','newsletter.enabled','participation.enabled',
  'referral.enabled'
]::text[])
  AND jsonb_typeof(s.value) IN ('string','boolean')
  AND NOT EXISTS (SELECT 1 FROM feature_flags f WHERE f.key = s.key)
ON CONFLICT (key) DO NOTHING;

-- 3) Legacy extension setting keys → their feature_flags equivalents
--    (knownExtensions SettingKey → FeatureFlagKey mapping).
INSERT INTO feature_flags (key, enabled, updated_at)
SELECT v.flag_key, COALESCE((s.value #>> '{}')::boolean, true), now()
FROM (VALUES
  ('extensions.blog.enabled',      'feature.blog'),
  ('extensions.webhooks.enabled',  'webhooks.enabled'),
  ('extensions.ai.enabled',        'feature.ai_content_gen'),
  ('extensions.affiliate.enabled', 'feature.affiliate_program')
) AS v(setting_key, flag_key)
JOIN system_settings s ON s.key = v.setting_key
WHERE jsonb_typeof(s.value) IN ('string','boolean')
ON CONFLICT (key) DO NOTHING;
