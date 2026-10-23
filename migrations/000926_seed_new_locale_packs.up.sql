-- 000926_seed_new_locale_packs
-- Provisions six additional locale packs (de, pt, it, nl, ar, zh) as
-- copy-English baselines — the same provisioning pattern used for es in
-- 000925. Values stay English until translated via the admin AI-translate
-- flow; the runtime-pack loading wired into the frontend stores already
-- layers every pack over English so partial coverage degrades gracefully.
-- lang.* self-descriptive keys are excluded from the copy and inserted with
-- correct per-locale metadata (en -> lang.en.*, fr -> lang.fr.*).
-- Also enables the new locales in i18n.enabled_languages so the SSR hooks
-- accept them (cookie + /xx/ URL prefixes).
BEGIN;

INSERT INTO translations (id, locale, "key", value)
SELECT gen_random_uuid(), 'de', "key", value
FROM translations
WHERE locale = 'en' AND "key" NOT LIKE 'lang.%'
ON CONFLICT (locale, "key") DO NOTHING;

INSERT INTO translations (id, locale, "key", value)
SELECT gen_random_uuid(), 'pt', "key", value
FROM translations
WHERE locale = 'en' AND "key" NOT LIKE 'lang.%'
ON CONFLICT (locale, "key") DO NOTHING;

INSERT INTO translations (id, locale, "key", value)
SELECT gen_random_uuid(), 'it', "key", value
FROM translations
WHERE locale = 'en' AND "key" NOT LIKE 'lang.%'
ON CONFLICT (locale, "key") DO NOTHING;

INSERT INTO translations (id, locale, "key", value)
SELECT gen_random_uuid(), 'nl', "key", value
FROM translations
WHERE locale = 'en' AND "key" NOT LIKE 'lang.%'
ON CONFLICT (locale, "key") DO NOTHING;

INSERT INTO translations (id, locale, "key", value)
SELECT gen_random_uuid(), 'ar', "key", value
FROM translations
WHERE locale = 'en' AND "key" NOT LIKE 'lang.%'
ON CONFLICT (locale, "key") DO NOTHING;

INSERT INTO translations (id, locale, "key", value)
SELECT gen_random_uuid(), 'zh', "key", value
FROM translations
WHERE locale = 'en' AND "key" NOT LIKE 'lang.%'
ON CONFLICT (locale, "key") DO NOTHING;

INSERT INTO translations (id, locale, "key", value)
VALUES
    (gen_random_uuid(), 'de', 'lang.de.enabled', 'true'),
    (gen_random_uuid(), 'de', 'lang.de.is_default', 'false'),
    (gen_random_uuid(), 'de', 'lang.de.name', 'German'),
    (gen_random_uuid(), 'de', 'lang.de.native_name', 'Deutsch'),
    (gen_random_uuid(), 'pt', 'lang.pt.enabled', 'true'),
    (gen_random_uuid(), 'pt', 'lang.pt.is_default', 'false'),
    (gen_random_uuid(), 'pt', 'lang.pt.name', 'Portuguese'),
    (gen_random_uuid(), 'pt', 'lang.pt.native_name', 'Português'),
    (gen_random_uuid(), 'it', 'lang.it.enabled', 'true'),
    (gen_random_uuid(), 'it', 'lang.it.is_default', 'false'),
    (gen_random_uuid(), 'it', 'lang.it.name', 'Italian'),
    (gen_random_uuid(), 'it', 'lang.it.native_name', 'Italiano'),
    (gen_random_uuid(), 'nl', 'lang.nl.enabled', 'true'),
    (gen_random_uuid(), 'nl', 'lang.nl.is_default', 'false'),
    (gen_random_uuid(), 'nl', 'lang.nl.name', 'Dutch'),
    (gen_random_uuid(), 'nl', 'lang.nl.native_name', 'Nederlands'),
    (gen_random_uuid(), 'ar', 'lang.ar.enabled', 'true'),
    (gen_random_uuid(), 'ar', 'lang.ar.is_default', 'false'),
    (gen_random_uuid(), 'ar', 'lang.ar.name', 'Arabic'),
    (gen_random_uuid(), 'ar', 'lang.ar.native_name', 'العربية'),
    (gen_random_uuid(), 'zh', 'lang.zh.enabled', 'true'),
    (gen_random_uuid(), 'zh', 'lang.zh.is_default', 'false'),
    (gen_random_uuid(), 'zh', 'lang.zh.name', 'Chinese'),
    (gen_random_uuid(), 'zh', 'lang.zh.native_name', '中文')
ON CONFLICT (locale, "key") DO UPDATE SET value = EXCLUDED.value;

UPDATE system_settings
SET value = '"en,fr,yo,es,de,pt,it,nl,ar,zh"'::jsonb,
    updated_at = now()
WHERE key = 'i18n.enabled_languages';

COMMIT;
