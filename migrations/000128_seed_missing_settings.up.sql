-- Seed system_settings keys missing from seed_data (AUDIT-BE-009)
INSERT INTO system_settings (tenant_id, key, value) VALUES
(1, 'feed.card_style', 'reddit'),
(1, 'content.link_preview.blacklist', '[]'),
(1, 'cors.allowed_origins', '*')
ON CONFLICT (tenant_id, key) DO NOTHING;
