-- Seed system_settings keys missing from seed_data (AUDIT-BE-009)
INSERT INTO system_settings (tenant_id, key, value) VALUES
('00000000-0000-0000-0000-000000000001', 'feed.card_style', '"reddit"'),
('00000000-0000-0000-0000-000000000001', 'content.link_preview.blacklist', '[]'),
('00000000-0000-0000-0000-000000000001', 'cors.allowed_origins', '"*"')
ON CONFLICT (tenant_id, key) DO NOTHING;
