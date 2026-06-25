# Section 18 — i18n / Language Packs

**Status:** ✅ Locked (French re-added per session-4 decision; earlier English-only "correction" in this session was an error — see ADR-013)

## Decided

English unprefixed at root; French at `/fr/...` prefix. V1 ships with two languages: `en` (default, `en-NG` posture) and `fr` (French). Further languages (Yoruba, Pidgin, additional regional English variants) are deferred until real sustained community demand — the admin Language Pack system supports them whenever that point comes.

Runtime-resolved translation (not Paraglide's compile-time bundling — see ADR-013 for why this was reversed mid-session) via a svelte-i18n-style runtime loader. Dictionaries cached in Valkey per locale.

## Schema

```sql
CREATE TABLE languages (
  code      text PRIMARY KEY,  -- 'en', 'fr', 'yo', etc.
  name      text NOT NULL,
  native_name text NOT NULL,
  is_rtl    boolean NOT NULL DEFAULT false,
  enabled   boolean NOT NULL DEFAULT false,
  is_default boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE translation_keys (
  key       text PRIMARY KEY,  -- e.g. 'nav.home', 'forum.solved', 'errors.not_found'
  namespace text NOT NULL,     -- 'common'|'nav'|'forum'|'blog'|'auth'|'admin'|...
  default_en text NOT NULL,    -- fallback if translation missing
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE translations (
  key           text NOT NULL REFERENCES translation_keys(key),
  language_code text NOT NULL REFERENCES languages(code),
  value         text NOT NULL,
  ai_generated  boolean NOT NULL DEFAULT false,
  human_reviewed boolean NOT NULL DEFAULT false,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (key, language_code)
);

-- Scoped assignment: which UserGroups manage which language's translations
CREATE TABLE language_pack_managers (
  language_code  text NOT NULL REFERENCES languages(code),
  user_group_id  uuid NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
  PRIMARY KEY (language_code, user_group_id)
);
```

## Admin Language Pack Workflow

```
1. Admin creates a new Language Pack (e.g. French)
2. AI auto-populate: platform_agent's 'platform.translate_string' tool fills all keys
   → translation rows inserted with ai_generated=true, human_reviewed=false
3. Admin assigns a UserGroup as Language Pack Managers for this language
4. Managers review/edit strings in Admin Panel → Settings → Translations
   (filterable by namespace, search by key, shows EN source + current translation)
5. Admin publishes at their discretion — no automatic completeness threshold
6. Missing keys at runtime always fall back to English, never a blank string
```

## Runtime Resolution

```ts
// Cached in Valkey: jc:i18n:{locale} → JSON dict, TTL 1h
async function getCachedDictionary(locale: string): Promise<Record<string, string>> {
  const cached = await valkey.get(`jc:i18n:${locale}`);
  if (cached) return JSON.parse(cached);
  const dict = await db.query.translations.findMany({ where: eq(translations.language_code, locale) });
  const result = Object.fromEntries(dict.map(r => [r.key, r.value]));
  await valkey.set(`jc:i18n:${locale}`, JSON.stringify(result), { ex: 3600 });
  return result;
}

// Translation function with EN fallback
function t(key: string, dict: Record<string, string>, en: Record<string, string>): string {
  return dict[key] ?? en[key] ?? `[${key}]`;
}
```

## hreflang Tags

Every public page emits hreflang tags for `en` and `fr`:

```html
<link rel="alternate" hreflang="en" href="https://japaconnect.com/path" />
<link rel="alternate" hreflang="fr" href="https://japaconnect.com/fr/path" />
<link rel="alternate" hreflang="x-default" href="https://japaconnect.com/path" />
```

## See Also

ADR-013 (Paraglide reversal), ADR-046.
