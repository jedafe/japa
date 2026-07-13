DELETE FROM system_settings WHERE tenant_id = 1 AND key IN ('feed.card_style', 'content.link_preview.blacklist', 'cors.allowed_origins');
