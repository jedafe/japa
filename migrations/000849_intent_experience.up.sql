-- 000849_intent_experience.up.sql
-- ADR-128: Intent Experience Personalization.
--
-- 1. geo_personalization.intent_matrix — per-intent experience rules:
--    hide_paths/pin_paths are segment-aware URL prefixes applied to
--    resolved menus (GET /menus/{slug}); demote_space_types are
--    space_types.slug values ordered last in /suggestions/spaces and
--    de-scored in onboarding recommendations. Gated by the
--    geo_personalization.enabled feature flag.
-- 2. geo_personalization.stage_tool_matrix gains {intent}:* keys so tool
--    recommendations key on the ADR-122 journey class ahead of the
--    ADR-064 archetype.

BEGIN;

INSERT INTO system_settings (key, value, is_public)
SELECT 'geo_personalization.intent_matrix', '{
  "visit":    {"hide_paths": ["/universities", "/scholarships", "/academy", "/tools/credential-checker", "/tools/language-test"], "pin_paths": ["/tools/travel", "/wiki", "/scam-alerts", "/services"], "demote_space_types": ["university"]},
  "business": {"hide_paths": ["/universities", "/scholarships", "/academy", "/tools/language-test"], "pin_paths": ["/services", "/tools/travel", "/tools/tax-calculator", "/tools/remittance"], "demote_space_types": ["university"]},
  "study":    {"pin_paths": ["/universities", "/scholarships", "/academy", "/tools/credential-checker", "/tools/language-test"]},
  "work":     {"pin_paths": ["/jobs", "/tools/cv-formatter", "/tools/credential-checker"]},
  "family":   {"pin_paths": ["/planning-groups", "/mutual-aid", "/community-locator"]}
}'::jsonb, false
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE system_settings.key = 'geo_personalization.intent_matrix');

-- stage_tool_matrix: intent-scoped keys resolve ahead of archetype keys.
UPDATE system_settings
SET value = value || '{
  "visit:*":    ["destination-wiki", "embassy-alerter", "visa-route-comparison", "scam-alert", "community-locator"],
  "business:*": ["service-directory", "destination-wiki", "tax-calculator", "community-locator"],
  "study:*":    ["credential-checker", "language-test-hub", "japa-academy", "visa-route-comparison"],
  "work:*":     ["cv-formatter", "credential-checker", "visa-route-comparison", "community-locator"]
}'::jsonb,
    updated_at = now()
WHERE key = 'geo_personalization.stage_tool_matrix';

COMMIT;
