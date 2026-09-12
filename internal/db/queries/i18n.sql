-- Internationalization (i18n) translation system

-- name: ListTranslationsByLocale :many
SELECT id, locale, "key", value, context, created_at, updated_at
FROM translations
WHERE locale = @locale
ORDER BY "key";

-- name: ListTranslationsByKeyPrefix :many
SELECT id, locale, "key", value, context, created_at, updated_at
FROM translations
WHERE "key" LIKE @prefix || '%'
ORDER BY "key";

-- name: GetTranslation :one
SELECT id, locale, "key", value, context, created_at, updated_at
FROM translations
WHERE locale = @locale AND "key" = @key;

-- name: UpsertTranslation :exec
INSERT INTO translations (locale, "key", value, context) VALUES (@locale, @key, @value, @context)

ON CONFLICT (locale, "key")
DO UPDATE SET value = EXCLUDED.value, context = EXCLUDED.context, updated_at = now();

-- name: DeleteTranslation :exec
DELETE FROM translations
WHERE id = @id;

-- name: DeleteTranslationsByLocale :exec
DELETE FROM translations
WHERE locale = @locale;

-- name: GetTranslationByID :one
SELECT id, locale, "key", value, context, created_at, updated_at
FROM translations
WHERE id = @id;

-- name: ListTranslationLocales :many
SELECT DISTINCT locale
FROM translations

ORDER BY locale;

-- name: CountTranslationsByLocale :many
SELECT locale, COUNT(*)::int AS count
FROM translations

GROUP BY locale
ORDER BY locale;
