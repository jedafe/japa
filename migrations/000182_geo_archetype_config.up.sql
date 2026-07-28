-- 000182_geo_archetype_config.up.sql
-- ADR-064 / Phase 38.03: Config-driven archetype market lists + the
-- destination_country custom field used by the VisitorContext resolver.
--
-- The archetype resolver (internal/visitor) classifies each visitor as
-- origin_visitor | diaspora | global_traveler based on whether their
-- current/origin country is a known "origin market" or "destination market".
-- ADR-064 Consequences: "Archetype market lists (origin vs destination
-- countries) are heuristics that will need tuning; keep them in config, not
-- hardcoded." These two system_settings keys hold CSVs of ISO 3166-1 alpha-2
-- codes so ops can tune the lists without a deploy.
--
-- Ref: japaconnect-engineering/adr/ADR-064-geo-aware-personalization.md
--      japaconnect-engineering/roadmaps/ROADMAP-BACKEND.md (Task 38.03)

-- 1. destination_country custom field in the user_profile_fields group.
--    Stored as a 2-letter ISO code in value_text. Text type (not select) so
--    the country list is not hardcoded here — the frontend renders a country
--    picker sourced from the spaces API (ADR-005 geo-first spaces).
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, is_system, is_required, position)
VALUES (
    '00000000-0000-0000-0000-000000000050',
    '00000000-0000-0000-0000-000000000040',
    '00000000-0000-0000-0000-000000000001',
    'Destination Country',
    'destination_country',
    'text',
    true,
    false,
    1
)
ON CONFLICT (id) DO NOTHING;

COMMENT ON COLUMN custom_fields.key IS
    'destination_country: ISO 3166-1 alpha-2 code of the country the user is relocating to (ADR-064 VisitorContext).';

-- 2. Config-driven archetype market lists (CSV of ISO codes).
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'geo_personalization.origin_markets',
     '"NG,GH,ZA,KE,EG"'),
    ('00000000-0000-0000-0000-000000000001', 'geo_personalization.destination_markets',
     '"GB,CA,US,DE,AU,AE"')
ON CONFLICT (tenant_id, key) DO NOTHING;
