-- remittance.sql — Remittance Rate Tracker queries (Section 74, ADR-075).
-- Covers provider CRUD, rate insertion/query, rate alert CRUD, and
-- admin operations.
-- Ref: japaconnect-engineering/sections/74-remittance-tracker.md
--      japaconnect-engineering/adr/ADR-075-remittance-rate-tracker.md

-- =====================================================================
-- Providers
-- =====================================================================

-- name: CreateRemittanceProvider :one
INSERT INTO remittance_providers (name, slug, logo_media_id, website_url, data_source, api_config, is_active, is_benchmark, display_order) VALUES (@name, @slug, sqlc.narg(logo_media_id), sqlc.narg(website_url), @data_source, sqlc.narg(api_config), @is_active, sqlc.narg(is_benchmark), @display_order)

RETURNING *;

-- name: UpdateRemittanceProvider :one
UPDATE remittance_providers
SET
 name = COALESCE(sqlc.narg(name), name),
 website_url = COALESCE(sqlc.narg(website_url), website_url),
 api_config = COALESCE(sqlc.narg(api_config), api_config),
 is_active = COALESCE(sqlc.narg(is_active), is_active),
 is_benchmark = COALESCE(sqlc.narg(is_benchmark), is_benchmark),
 display_order = COALESCE(sqlc.narg(display_order), display_order)
WHERE id = @id
RETURNING *;

-- name: GetRemittanceProviderByID :one
SELECT * FROM remittance_providers
WHERE id = @id;

-- name: ListRemittanceProviders :many
SELECT * FROM remittance_providers
WHERE (@active_only::boolean = false OR is_active = true)
ORDER BY display_order ASC, name ASC;

-- name: ListRemittanceProvidersPaginated :many
SELECT * FROM remittance_providers
WHERE (@active_only::boolean = false OR is_active = true)
ORDER BY display_order ASC, name ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountRemittanceProviders :one
SELECT COUNT(*) AS count
FROM remittance_providers
WHERE (@active_only::boolean = false OR is_active = true);

-- =====================================================================
-- Rates
-- =====================================================================

-- name: InsertRemittanceRate :one
INSERT INTO remittance_rates (provider_id, send_currency, receive_currency, rate, fee_flat, fee_pct, transfer_time, is_manual, source_url) VALUES (@provider_id, @send_currency, @receive_currency, @rate, sqlc.narg(fee_flat), sqlc.narg(fee_pct), sqlc.narg(transfer_time), @is_manual, sqlc.narg(source_url))

RETURNING *;

-- name: GetLatestRatesForCorridor :many
SELECT DISTINCT ON (rr.provider_id)
 rr.*,
 rp.name AS provider_name,
 rp.slug AS provider_slug,
 rp.website_url AS provider_website,
 rp.is_benchmark AS provider_is_benchmark
FROM remittance_rates rr
JOIN remittance_providers rp ON rp.id = rr.provider_id
WHERE rr.send_currency = @send_currency
 AND rr.receive_currency = @receive_currency
 AND rp.is_active = true
ORDER BY rr.provider_id, rr.fetched_at DESC;

-- name: GetRateHistory :many
SELECT
 date_trunc('day', fetched_at)::date AS day,
 MAX(rate) AS max_rate,
 MIN(rate) AS min_rate,
 AVG(rate)::numeric(18,6) AS avg_rate
FROM remittance_rates
WHERE send_currency = @send_currency
 AND receive_currency = @receive_currency
 AND fetched_at >= @from_date
GROUP BY day
ORDER BY day ASC;

-- name: GetCorridors :many
SELECT DISTINCT
 send_currency,
 receive_currency
FROM remittance_rates

ORDER BY send_currency, receive_currency;

-- name: PruneOldRates :exec
-- Keep the most recently fetched rate per (provider, corridor, day); uuid
-- has no ordering so MIN(id) is invalid — DISTINCT ON picks the latest row.
DELETE FROM remittance_rates
WHERE remittance_rates.fetched_at < @cutoff_date
 AND remittance_rates.id NOT IN (
 SELECT DISTINCT ON (provider_id, send_currency, receive_currency, date_trunc('day', fetched_at))
        remittance_rates.id
 FROM remittance_rates
 WHERE remittance_rates.fetched_at < @cutoff_date
 ORDER BY provider_id, send_currency, receive_currency, date_trunc('day', fetched_at), fetched_at DESC
 );

-- =====================================================================
-- Rate Alerts
-- =====================================================================

-- name: CreateRateAlert :one
INSERT INTO remittance_rate_alerts (user_id, send_currency, receive_currency, target_rate, direction) VALUES (@user_id, @send_currency, @receive_currency, @target_rate, @direction)

ON CONFLICT (user_id, send_currency, receive_currency, direction) DO NOTHING
RETURNING *;

-- name: UpdateRateAlert :one
UPDATE remittance_rate_alerts
SET
 target_rate = COALESCE(sqlc.narg(target_rate), target_rate),
 direction = COALESCE(sqlc.narg(direction), direction),
 is_active = COALESCE(sqlc.narg(is_active), is_active),
 last_triggered_at = COALESCE(sqlc.narg(last_triggered_at), last_triggered_at)
WHERE id = @id AND user_id = @user_id
RETURNING *;

-- name: DeleteRateAlert :exec
DELETE FROM remittance_rate_alerts
WHERE id = @id AND user_id = @user_id;

-- name: GetRateAlertByID :one
SELECT * FROM remittance_rate_alerts
WHERE id = @id AND user_id = @user_id;

-- name: ListUserRateAlerts :many
SELECT * FROM remittance_rate_alerts
WHERE user_id = @user_id
 AND (@active_only::boolean = false OR is_active = true)
ORDER BY created_at DESC;

-- name: ListActiveAlertsForCorridor :many
SELECT * FROM remittance_rate_alerts
WHERE send_currency = @send_currency
 AND receive_currency = @receive_currency
 AND is_active = true
 AND (last_triggered_at IS NULL OR last_triggered_at < now() - interval '24 hours');

-- name: ListAllActiveAlerts :many
SELECT * FROM remittance_rate_alerts
WHERE is_active = true
 AND (last_triggered_at IS NULL OR last_triggered_at < now() - interval '24 hours')
ORDER BY created_at DESC;

-- name: MarkAlertTriggered :exec
UPDATE remittance_rate_alerts
SET last_triggered_at = now()
WHERE id = @id;
