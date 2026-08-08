-- 000247_geo_awareness_extension.up.sql
-- ADR-064a: Geo-Awareness Extension — origin_country custom field,
-- stage_tool_matrix system setting, city_level_enabled killswitch,
-- Nominatim reverse-geocoding config, and tool label/URL catalog.
--
-- This migration adds:
-- 1. origin_country custom field (mirrors destination_country pattern
--    from migration 000182) — ISO 3166-1 alpha-2 code of the user's
--    home/origin country. Used by VisitorContext resolver (ADR-064a).
-- 2. geo_personalization.stage_tool_matrix — JSON config mapping
--    archetype + japa_stage → recommended tool/feature slugs.
-- 3. geo_personalization.city_level_enabled — killswitch for city-level
--    personalization features (default: true).
-- 4. geo_personalization.nominatim_url — configurable reverse-geocoding
--    endpoint (Rule 3: no hardcoded URLs).
-- 5. geo_personalization.nominatim_timeout_ms — configurable request
--    timeout in milliseconds (Rule 3: no hardcoded limits).
-- 6. geo_personalization.tool_labels — JSON slug→label map for the
--    recommendation engine (Rule 3: no hardcoded display strings).
-- 7. geo_personalization.tool_urls — JSON slug→URL map for the
--    recommendation engine (Rule 3: no hardcoded URLs).
--
-- Ref: japaconnect-engineering/adr/ADR-064a-geo-awareness-extension.md

-- 1. origin_country custom field in the user_profile_fields group.
--    Same pattern as destination_country (migration 000182): text type
--    so the country list is sourced from the spaces API, not hardcoded.
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, is_system, is_required, position)
VALUES (
    '00000000-0000-0000-0000-000000000051',
    '00000000-0000-0000-0000-000000000040',
    '00000000-0000-0000-0000-000000000001',
    'Origin Country',
    'origin_country',
    'text',
    true,
    false,
    2
)
ON CONFLICT (id) DO NOTHING;

-- 2. stage_tool_matrix: JSON mapping archetype + japa_stage → tool/feature slugs.
--    The recommendation engine (GET /api/v1/recommendations) reads this config
--    to determine which tools to surface for a given VisitorContext.
--    Keys are "{archetype}:{japa_stage}" or "{archetype}:*" for all stages.
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'geo_personalization.stage_tool_matrix',
     '"{
  \"origin_visitor:planning\": [\"visa-route-comparison\", \"embassy-alerter\", \"processing-time-tracker\", \"credential-checker\"],
  \"origin_visitor:considering\": [\"destination-wiki\", \"cost-of-living-comparison\", \"community-locator\"],
  \"origin_visitor:newly_japa\": [\"arrival-checklist\", \"housing-guide\", \"banking-guide\", \"language-test-hub\"],
  \"origin_visitor:settling_in\": [\"arrival-checklist\", \"housing-guide\", \"attestation-guide\", \"community-locator\"],
  \"origin_visitor:*\": [\"destination-wiki\", \"visa-route-comparison\", \"embassy-alerter\"],
  \"diaspora:settling_in\": [\"arrival-checklist\", \"housing-guide\", \"banking-guide\", \"language-test-hub\"],
  \"diaspora:settled\": [\"tax-calculator\", \"attestation-guide\", \"community-locator\", \"mutual-aid-board\"],
  \"diaspora:*\": [\"community-locator\", \"destination-wiki\", \"service-directory\"],
  \"global_traveler:*\": [\"japa-university\", \"destination-wiki\", \"community-locator\"],
  \"*:planning\": [\"visa-route-comparison\", \"embassy-alerter\", \"processing-time-tracker\"],
  \"*:considering\": [\"destination-wiki\", \"cost-of-living-comparison\"],
  \"*:newly_japa\": [\"arrival-checklist\", \"housing-guide\"],
  \"*:settling_in\": [\"arrival-checklist\", \"housing-guide\", \"banking-guide\"],
  \"*:settled\": [\"tax-calculator\", \"attestation-guide\", \"community-locator\"],
  \"*:*\": [\"destination-wiki\", \"community-locator\"]
}"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- 3. city_level_enabled killswitch (default: true).
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'geo_personalization.city_level_enabled', 'true')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- 4. nominatim_url — configurable reverse-geocoding endpoint (Rule 3).
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'geo_personalization.nominatim_url',
     'https://nominatim.openstreetmap.org/reverse')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- 5. nominatim_timeout_ms — configurable request timeout (Rule 3).
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'geo_personalization.nominatim_timeout_ms', '5000')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- 6. tool_labels — JSON slug→label map for the recommendation engine (Rule 3).
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'geo_personalization.tool_labels',
     '"{
  \"visa-route-comparison\": \"Visa Route Comparison\",
  \"embassy-alerter\": \"Embassy Appointment Alerter\",
  \"processing-time-tracker\": \"Processing Time Tracker\",
  \"credential-checker\": \"Credential Checker\",
  \"cv-formatter\": \"CV Formatter\",
  \"housing-guide\": \"Housing Guide\",
  \"attestation-guide\": \"Attestation Guide\",
  \"language-test-hub\": \"Language Test Hub\",
  \"tax-calculator\": \"Tax Calculator\",
  \"community-locator\": \"Community Locator\",
  \"destination-wiki\": \"Destination Wiki\",
  \"cost-of-living-comparison\": \"Cost of Living Comparison\",
  \"arrival-checklist\": \"Arrival Checklist\",
  \"banking-guide\": \"Banking Guide\",
  \"mutual-aid-board\": \"Mutual Aid Board\",
  \"service-directory\": \"Service Directory\",
  \"japa-university\": \"Japa University\",
  \"story-wall\": \"Community Story Wall\",
  \"scam-alert\": \"Scam Alert\",
  \"planning-groups\": \"Planning Groups\",
  \"visa-timeline\": \"Visa Timeline Tracker\",
  \"community-calendar\": \"Community Calendar\",
  \"japa-stage\": \"Japa Stage Profiles\",
  \"ask-an-expat\": \"Ask an Expat AMA\",
  \"achievement-feed\": \"Achievement Feed\",
  \"mentorship\": \"Mentorship Matching\"
}"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- 7. tool_urls — JSON slug→URL map for the recommendation engine (Rule 3).
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'geo_personalization.tool_urls',
     '"{
  \"visa-route-comparison\": \"/tools/visa-route-comparison\",
  \"embassy-alerter\": \"/tools/embassy-alerter\",
  \"processing-time-tracker\": \"/tools/processing-time-tracker\",
  \"credential-checker\": \"/tools/credential-checker\",
  \"cv-formatter\": \"/tools/cv-formatter\",
  \"housing-guide\": \"/tools/housing-guide\",
  \"attestation-guide\": \"/tools/attestation-guide\",
  \"language-test-hub\": \"/tools/language-test-hub\",
  \"tax-calculator\": \"/tools/tax-calculator\",
  \"community-locator\": \"/tools/community-locator\",
  \"destination-wiki\": \"/wiki\",
  \"cost-of-living-comparison\": \"/tools/cost-of-living\",
  \"arrival-checklist\": \"/tools/arrival-checklist\",
  \"banking-guide\": \"/tools/banking-guide\",
  \"mutual-aid-board\": \"/mutual-aid\",
  \"service-directory\": \"/services\",
  \"japa-university\": \"/university\",
  \"story-wall\": \"/stories\",
  \"scam-alert\": \"/scam-alerts\",
  \"planning-groups\": \"/groups\",
  \"visa-timeline\": \"/tools/visa-timeline\",
  \"community-calendar\": \"/calendar\",
  \"japa-stage\": \"/japa-stage\",
  \"ask-an-expat\": \"/ama\",
  \"achievement-feed\": \"/achievements\",
  \"mentorship\": \"/mentorship\"
}"')
ON CONFLICT (tenant_id, key) DO NOTHING;
