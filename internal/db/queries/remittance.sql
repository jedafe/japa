-- remittance.sql — Remittance Rate Tracker queries (Section 74, ADR-075).
-- Covers provider CRUD, rate insertion/query, rate alert CRUD, and
-- admin operations.
-- Ref: japaconnect-engineering/sections/74-remittance-tracker.md
--      japaconnect-engineering/adr/ADR-075-remittance-rate-tracker.md

-- =====================================================================
-- Providers
-- =====================================================================

-- name: CreateRemittanceProvider :one
INSERT INTO remittance_providers (tenant_id, name, slug, logo_media_id, website_url, data_source, api_config, is_active, display_order)
VALUES (@tenant_id, @name, @slug, sqlc.narg(logo_media_id), sqlc.narg(website_url), @data_source, sqlc.narg(api_config), @is_active, @display_order)
RETURNING *;

-- name: UpdateRemittanceProvider :one
UPDATE remittance_providers
SET
    name          = COALESCE(sqlc.narg(name), name),
    website_url   = COALESCE(sqlc.narg(website_url), website_url),
    api_config    = COALESCE(sqlc.narg(api_config), api_config),
    is_active     = COALESCE(sqlc.narg(is_active), is_active),
    display_order = COALESCE(sqlc.narg(display_order), display_order)
WHERE tenant_id = @tenant_id AND id = @id
RETURNING *;

-- name: GetRemittanceProviderByID :one
SELECT * FROM remittance_providers
WHERE tenant_id = @tenant_id AND id = @id;

-- name: ListRemittanceProviders :many
SELECT * FROM remittance_providers
WHERE tenant_id = @tenant_id
  AND (@active_only::boolean = false OR is_active = true)
ORDER BY display_order ASC, name ASC;

-- name: ListRemittanceProvidersPaginated :many
SELECT * FROM remittance_providers
WHERE tenant_id = @tenant_id
  AND (@active_only::boolean = false OR is_active = true)
ORDER BY display_order ASC, name ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountRemittanceProviders :one
SELECT COUNT(*) AS count
FROM remittance_providers
WHERE tenant_id = @tenant_id
  AND (@active_only::boolean = false OR is_active = true);

-- =====================================================================
-- Rates
-- =====================================================================

-- name: InsertRemittanceRate :one
INSERT INTO remittance_rates (
    tenant_id, provider_id, send_currency, receive_currency,
    rate, fee_flat, fee_pct, transfer_time, is_manual, source_url
)
VALUES (
    @tenant_id, @provider_id, @send_currency, @receive_currency,
    @rate, sqlc.narg(fee_flat), sqlc.narg(fee_pct), sqlc.narg(transfer_time),
    @is_manual, sqlc.narg(source_url)
)
RETURNING *;

-- name: GetLatestRatesForCorridor :many
SELECT DISTINCT ON (rr.provider_id)
    rr.*,
    rp.name   AS provider_name,
    rp.slug   AS provider_slug,
    rp.website_url AS provider_website
FROM remittance_rates rr
JOIN remittance_providers rp ON rp.id = rr.provider_id
WHERE rr.tenant_id = @tenant_id
  AND rr.send_currency = @send_currency
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
WHERE tenant_id = @tenant_id
  AND send_currency = @send_currency
  AND receive_currency = @receive_currency
  AND fetched_at >= @from_date
GROUP BY day
ORDER BY day ASC;

-- name: GetCorridors :many
SELECT DISTINCT
    send_currency,
    receive_currency
FROM remittance_rates
WHERE tenant_id = @tenant_id
ORDER BY send_currency, receive_currency;

-- name: PruneOldRates :exec
DELETE FROM remittance_rates
WHERE remittance_rates.tenant_id = @tenant_id
  AND remittance_rates.fetched_at < @cutoff_date
  AND remittance_rates.id NOT IN (
    SELECT MIN(remittance_rates.id) FROM remittance_rates
    WHERE remittance_rates.tenant_id = @tenant_id
      AND remittance_rates.fetched_at < @cutoff_date
    GROUP BY provider_id, send_currency, receive_currency, date_trunc('day', fetched_at)
  );

-- =====================================================================
-- Rate Alerts
-- =====================================================================

-- name: CreateRateAlert :one
INSERT INTO remittance_rate_alerts (tenant_id, user_id, send_currency, receive_currency, target_rate, direction)
VALUES (@tenant_id, @user_id, @send_currency, @receive_currency, @target_rate, @direction)
ON CONFLICT (tenant_id, user_id, send_currency, receive_currency, direction) DO NOTHING
RETURNING *;

-- name: UpdateRateAlert :one
UPDATE remittance_rate_alerts
SET
    target_rate      = COALESCE(sqlc.narg(target_rate), target_rate),
    direction        = COALESCE(sqlc.narg(direction), direction),
    is_active        = COALESCE(sqlc.narg(is_active), is_active),
    last_triggered_at = COALESCE(sqlc.narg(last_triggered_at), last_triggered_at)
WHERE tenant_id = @tenant_id AND id = @id AND user_id = @user_id
RETURNING *;

-- name: DeleteRateAlert :exec
DELETE FROM remittance_rate_alerts
WHERE tenant_id = @tenant_id AND id = @id AND user_id = @user_id;

-- name: GetRateAlertByID :one
SELECT * FROM remittance_rate_alerts
WHERE tenant_id = @tenant_id AND id = @id AND user_id = @user_id;

-- name: ListUserRateAlerts :many
SELECT * FROM remittance_rate_alerts
WHERE tenant_id = @tenant_id AND user_id = @user_id
  AND (@active_only::boolean = false OR is_active = true)
ORDER BY created_at DESC;

-- name: ListActiveAlertsForCorridor :many
SELECT * FROM remittance_rate_alerts
WHERE tenant_id = @tenant_id
  AND send_currency = @send_currency
  AND receive_currency = @receive_currency
  AND is_active = true
  AND (last_triggered_at IS NULL OR last_triggered_at < now() - interval '24 hours');

-- name: MarkAlertTriggered :exec
UPDATE remittance_rate_alerts
SET last_triggered_at = now()
WHERE id = @id;
