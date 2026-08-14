-- Internationalization (i18n) translation system

-- name: ListTranslationsByLocale :many
SELECT id, locale, "key", value, context, created_at, updated_at
FROM translations
WHERE tenant_id = @tenant_id AND locale = @locale
ORDER BY "key";

-- name: ListTranslationsByKeyPrefix :many
SELECT id, locale, "key", value, context, created_at, updated_at
FROM translations
WHERE tenant_id = @tenant_id AND "key" LIKE @prefix || '%'
ORDER BY "key";

-- name: GetTranslation :one
SELECT id, locale, "key", value, context, created_at, updated_at
FROM translations
WHERE tenant_id = @tenant_id AND locale = @locale AND "key" = @key;

-- name: UpsertTranslation :exec
INSERT INTO translations (tenant_id, locale, "key", value, context)
VALUES (@tenant_id, @locale, @key, @value, @context)
ON CONFLICT (tenant_id, locale, "key")
DO UPDATE SET value = EXCLUDED.value, context = EXCLUDED.context, updated_at = now();

-- name: DeleteTranslation :exec
DELETE FROM translations
WHERE tenant_id = @tenant_id AND id = @id;

-- name: DeleteTranslationsByLocale :exec
DELETE FROM translations
WHERE tenant_id = @tenant_id AND locale = @locale;

-- name: GetTranslationByID :one
SELECT id, locale, "key", value, context, created_at, updated_at
FROM translations
WHERE tenant_id = @tenant_id AND id = @id;

-- name: ListTranslationLocales :many
SELECT DISTINCT locale
FROM translations
WHERE tenant_id = @tenant_id
ORDER BY locale;

-- name: CountTranslationsByLocale :many
SELECT locale, COUNT(*)::int AS count
FROM translations
WHERE tenant_id = @tenant_id
GROUP BY locale
ORDER BY locale;
