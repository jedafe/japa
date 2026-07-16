-- Rollback: delete the 28 v2 control-app i18n keys seeded by the up migration.

DELETE FROM translations
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND locale = 'en'
  AND "key" IN (
    'settings.card_style_compact',
    'settings.card_style_grid',
    'settings.card_style_magazine',
    'settings.card_style_reddit',
    'settings.currency_eur',
    'settings.currency_gbp',
    'settings.currency_ngn',
    'settings.currency_usd',
    'settings.lang_en',
    'settings.lang_fr',
    'settings.lang_ha',
    'settings.lang_ig',
    'settings.lang_pcm',
    'settings.lang_yo',
    'settings.log_level_debug',
    'settings.log_level_error',
    'settings.log_level_info',
    'settings.log_level_warn',
    'settings.storage_azure',
    'settings.storage_gcs',
    'settings.storage_local',
    'settings.storage_s3',
    'settings.storage_seaweedfs',
    'settings.video_avi',
    'settings.video_mkv',
    'settings.video_mov',
    'settings.video_mp4',
    'settings.video_webm'
);
