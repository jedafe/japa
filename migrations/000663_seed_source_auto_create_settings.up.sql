BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'sources.auto_create_for_universities', to_jsonb(false)),
    ('00000000-0000-0000-0000-000000000001', 'sources.auto_create_for_organizations', to_jsonb(false)),
    ('00000000-0000-0000-0000-000000000001', 'sources.auto_create_for_users', to_jsonb(false)),
    ('00000000-0000-0000-0000-000000000001', 'intel.radar.mixed_fetch_page_multiplier', to_jsonb(10))
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
