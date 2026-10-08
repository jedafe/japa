-- 000811_task13_flag_backfill.up.sql
-- Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §3.3, §7
--
-- Task 13 completes the service-level migration: services now read
-- reachability via features.Checker.IsEnabled (feature_flags). Migration
-- 000807 backfilled the first cohort; this file covers the keys Task 13
-- uncovered that were NOT in that list, plus the rename mappings for
-- services whose reachability key differs from the legacy setting name.
--
-- EXPLICIT key lists only — sub-behavior switches (e.g.
-- intel.discovery.enabled, service_packages.fee_enabled,
-- wallet.credit_purchase_enabled, ai.content_assist_enabled) remain
-- system_settings and are deliberately excluded.

-- 1) Same-name backfill: feature flag key == system_settings key.
INSERT INTO feature_flags (key, enabled, updated_at)
SELECT s.key, COALESCE((s.value #>> '{}')::boolean, true), now()
FROM system_settings s
WHERE s.key = ANY(ARRAY[
  'video.enabled','list.enabled','studio.enabled','rag.enabled',
  'geo_personalization.enabled','push.enabled','visa_timeline.enabled',
  'tax_calculator.enabled','service_packages.enabled',
  'service_coupons.enabled'
]::text[])
  AND jsonb_typeof(s.value) IN ('string','boolean')
  AND NOT EXISTS (SELECT 1 FROM feature_flags f WHERE f.key = s.key)
ON CONFLICT (key) DO NOTHING;

-- 2) Rename mappings: the flag key the service now reads differs from the
--    legacy system_settings key. (000807 minted same-name
--    newsletter.enabled/referral.enabled flag rows — those are inert; the
--    services and route gates resolve feature.newsletter/feature.referral.)
INSERT INTO feature_flags (key, enabled, updated_at)
SELECT v.flag_key, COALESCE((s.value #>> '{}')::boolean, true), now()
FROM (VALUES
  ('ai.japabot_enabled', 'feature.japabot'),
  ('newsletter.enabled', 'feature.newsletter'),
  ('referral.enabled',   'feature.referral'),
  ('agent.enabled',      'feature.ai_agent')
) AS v(setting_key, flag_key)
JOIN system_settings s ON s.key = v.setting_key
WHERE jsonb_typeof(s.value) IN ('string','boolean')
ON CONFLICT (key) DO NOTHING;
