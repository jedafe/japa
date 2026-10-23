-- 000927_seed_yo_locale_and_parity_backfill
-- yo is listed in i18n.enabled_languages and PREFIXED_LOCALES but was never
-- provisioned a pack — seed it with the same copy-English baseline pattern
-- as the other locales (see 000925/000926), then run a cross-locale parity
-- backfill so every locale in the table carries every non-lang.* en key.
-- lang.* self-descriptive keys are excluded from copies and inserted with
-- correct per-locale metadata. Idempotent: safe to re-run.
BEGIN;

-- 1. Baseline pack for yo (copy of en minus lang.* metadata keys).
INSERT INTO translations (id, locale, "key", value)
SELECT gen_random_uuid(), 'yo', "key", value
FROM translations
WHERE locale = 'en' AND "key" NOT LIKE 'lang.%'
ON CONFLICT (locale, "key") DO NOTHING;

-- 2. Self-descriptive lang.yo.* metadata (each pack declares its own locale).
INSERT INTO translations (id, locale, "key", value)
VALUES
    (gen_random_uuid(), 'yo', 'lang.yo.enabled', 'true'),
    (gen_random_uuid(), 'yo', 'lang.yo.is_default', 'false'),
    (gen_random_uuid(), 'yo', 'lang.yo.name', 'Yoruba'),
    (gen_random_uuid(), 'yo', 'lang.yo.native_name', 'Yorùbá')
ON CONFLICT (locale, "key") DO UPDATE SET value = EXCLUDED.value;

-- 3. Parity backfill: every locale in the table gets every non-lang.* en key
-- it is missing (copy-en value until translated). This is the canonical
-- "seed all locales" pattern — every future i18n seed migration SHOULD end
-- with this statement so non-en/fr locales never drift behind again.
INSERT INTO translations (id, locale, "key", value)
SELECT gen_random_uuid(), l.locale, e."key", e.value
FROM translations e
CROSS JOIN (SELECT DISTINCT locale FROM translations) l
WHERE e.locale = 'en'
  AND e."key" NOT LIKE 'lang.%'
  AND NOT EXISTS (
      SELECT 1
      FROM translations x
      WHERE x.locale = l.locale AND x."key" = e."key"
  )
ON CONFLICT (locale, "key") DO NOTHING;

COMMIT;
