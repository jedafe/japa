-- name: ListActiveCurrencies :many
SELECT code, name, symbol, credits_per_unit, is_active, position, created_at, updated_at
FROM currencies
WHERE is_active = true
ORDER BY position ASC, code ASC;

-- name: ListAllCurrencies :many
SELECT code, name, symbol, credits_per_unit, is_active, position, created_at, updated_at
FROM currencies
ORDER BY position ASC, code ASC;

-- name: GetCurrencyByCode :one
SELECT code, name, symbol, credits_per_unit, is_active, position, created_at, updated_at
FROM currencies
WHERE code = @code;

-- name: CreateCurrency :one
INSERT INTO currencies (code, name, symbol, credits_per_unit, position) VALUES (@code, @name, @symbol, @credits_per_unit, @position)

RETURNING code, name, symbol, credits_per_unit, is_active, position, created_at, updated_at;

-- name: UpdateCurrency :one
UPDATE currencies
SET name = @name, symbol = @symbol, credits_per_unit = @credits_per_unit,
 is_active = @is_active, position = @position, updated_at = now()
WHERE code = @code
RETURNING code, name, symbol, credits_per_unit, is_active, position, created_at, updated_at;

-- name: DeactivateCurrency :exec
UPDATE currencies SET is_active = false, updated_at = now()
WHERE code = @code;

-- name: CountSpacesByCurrency :one
SELECT count(*)::int FROM spaces WHERE currency_code = @currency_code;

-- =====================
-- Credit Packs CRUD
-- =====================

-- name: ListActiveCreditPacks :many
SELECT id, name, description, credit_amount, bonus_credits, min_user_level,
 is_active, position, created_at, updated_at
FROM credit_packs
WHERE is_active = true
ORDER BY position ASC, created_at ASC;

-- name: ListAllCreditPacks :many
SELECT id, name, description, credit_amount, bonus_credits, min_user_level,
 is_active, position, created_at, updated_at
FROM credit_packs

ORDER BY position ASC, created_at ASC;

-- name: GetCreditPackByID :one
SELECT id, name, description, credit_amount, bonus_credits, min_user_level,
 is_active, position, created_at, updated_at
FROM credit_packs
WHERE id = @id;

-- name: CreateCreditPack :one
INSERT INTO credit_packs (name, description, credit_amount, bonus_credits, min_user_level, position) VALUES (@name, sqlc.narg(description), @credit_amount, @bonus_credits, sqlc.narg(min_user_level), @position)

RETURNING id, name, description, credit_amount, bonus_credits, min_user_level,
 is_active, position, created_at, updated_at;

-- name: UpdateCreditPack :one
UPDATE credit_packs
SET name = @name, description = sqlc.narg(description), credit_amount = @credit_amount,
 bonus_credits = @bonus_credits, min_user_level = sqlc.narg(min_user_level),
 is_active = @is_active, position = @position, updated_at = now()
WHERE id = @id
RETURNING id, name, description, credit_amount, bonus_credits, min_user_level,
 is_active, position, created_at, updated_at;

-- name: DeactivateCreditPack :exec
UPDATE credit_packs SET is_active = false, updated_at = now()
WHERE id = @id;

-- =====================
-- User Preferred Currency
-- =====================

-- name: GetUserPreferredCurrency :one
SELECT preferred_currency FROM user_profiles WHERE user_id = @user_id;

-- name: SetUserPreferredCurrency :exec
UPDATE user_profiles SET preferred_currency = sqlc.narg(preferred_currency)
WHERE user_id = @user_id;

-- =====================
-- Country → Currency resolution
-- =====================

-- name: GetCurrencyByCountryCode :one
SELECT c.code, c.name, c.symbol, c.credits_per_unit, c.is_active, c.position, c.created_at, c.updated_at
FROM currencies c
JOIN spaces s ON s.currency_code = c.code
WHERE s.geo_code = @geo_code AND s.level = 'country'
LIMIT 1;

-- =====================
-- Credit Purchase History (admin)
-- =====================

-- name: ListCreditPurchases :many
SELECT p.id, p.user_id, p.provider, p.amount, p.currency, p.status,
 p.created_at, p.updated_at,
 u.username, up.display_name
FROM payments p
LEFT JOIN users u ON u.id = p.user_id
LEFT JOIN user_profiles up ON up.user_id = p.user_id
WHERE p.purpose_type = 'credit_purchase'
 AND (sqlc.narg(user_id)::uuid IS NULL OR p.user_id = sqlc.narg(user_id))
 AND (sqlc.narg(currency)::text IS NULL OR p.currency = sqlc.narg(currency))
 AND (p.created_at >= sqlc.narg(from_date) OR sqlc.narg(from_date) IS NULL)
 AND (p.created_at <= sqlc.narg(to_date) OR sqlc.narg(to_date) IS NULL)
ORDER BY p.created_at DESC, p.id DESC
LIMIT @page_limit OFFSET @page_offset;
