-- 000926_seed_new_locale_packs (down)
BEGIN;

DELETE FROM translations
WHERE locale IN ('de', 'pt', 'it', 'nl', 'ar', 'zh');

UPDATE system_settings
SET value = '"en,fr,yo"'::jsonb,
    updated_at = now()
WHERE key = 'i18n.enabled_languages';

COMMIT;
