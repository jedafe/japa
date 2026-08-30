-- 000572_seed_spaces_settings.up.sql
-- Seeds public space settings so the Main app can render level filters,
-- page sizes, permission axes, and the default member-created space type
-- from system_settings instead of hardcoded values (Rule 3 / Rule 16).
--
-- Default tenant UUID: '00000000-0000-0000-0000-000000000001'
-- Ref: japaconnect-engineering/sections/05-geo-first-space-system.md
--      japaconnect-engineering/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (tenant_id, key, value, is_public) VALUES
    ('00000000-0000-0000-0000-000000000001', 'spaces.levels', to_jsonb(ARRAY['country','state','city','custom']::text[]), true),
    ('00000000-0000-0000-0000-000000000001', 'spaces.page_size', to_jsonb(24), true),
    ('00000000-0000-0000-0000-000000000001', 'spaces.hub_page_size', to_jsonb(20), true),
    ('00000000-0000-0000-0000-000000000001', 'spaces.forum_page_size', to_jsonb(50), true),
    ('00000000-0000-0000-0000-000000000001', 'spaces.permission_axes', to_jsonb('{"visibility": ["public", "members", "private"], "posting": ["open", "members", "verified", "moderators", "admins"], "joining": ["open", "request", "invite"]}'::json), true),
    ('00000000-0000-0000-0000-000000000001', 'spaces.default_member_space_type_slug', to_jsonb('community'::text), true)
ON CONFLICT (tenant_id, key) DO UPDATE SET value = EXCLUDED.value, is_public = true;

COMMIT;
