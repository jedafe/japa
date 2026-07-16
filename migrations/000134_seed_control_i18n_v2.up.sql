-- Seed 28 control-app i18n keys not covered by 000133 (v2 additions).
-- Trimmed from the original v2 dump: the other 498 keys were byte-identical
-- duplicates of 000133 and inserted nothing under ON CONFLICT DO NOTHING.

INSERT INTO translations (tenant_id, locale, "key", value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.card_style_compact', 'Compact'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.card_style_grid', 'Grid'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.card_style_magazine', 'Magazine'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.card_style_reddit', 'Reddit'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.currency_eur', 'Euro (EUR)'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.currency_gbp', 'British Pound (GBP)'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.currency_ngn', 'Nigerian Naira (NGN)'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.currency_usd', 'US Dollar (USD)'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.lang_en', 'English'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.lang_fr', 'Francais'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.lang_ha', 'Hausa'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.lang_ig', 'Igbo'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.lang_pcm', 'Nigerian Pidgin'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.lang_yo', 'Yoruba'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.log_level_debug', 'Debug'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.log_level_error', 'Error'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.log_level_info', 'Info'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.log_level_warn', 'Warn'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.storage_azure', 'Azure Blob'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.storage_gcs', 'Google Cloud Storage'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.storage_local', 'Local Disk'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.storage_s3', 'Amazon S3'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.storage_seaweedfs', 'SeaweedFS'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.video_avi', 'AVI'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.video_mkv', 'MKV'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.video_mov', 'MOV'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.video_mp4', 'MP4'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'settings.video_webm', 'WebM')
ON CONFLICT (tenant_id, locale, "key") DO NOTHING;
