-- fx.sql — FX Currency Converter queries (Section 112, ADR-125).
-- Mid-market base→quote matrix: insert, latest-per-quote reads, fetch-time
-- probe (adapter throttle), picker currency list, retention prune.
-- Ref: japaconnect-engineering/sections/112-fx-converter.md
--      japaconnect-engineering/adr/ADR-125-fx-converter.md

-- name: InsertFxRate :one
INSERT INTO fx_rates (base_currency, quote_currency, rate, source)
VALUES (@base_currency, @quote_currency, @rate, sqlc.narg(source))
RETURNING *;

-- name: GetLatestFxMatrix :many
-- Latest rate per quote currency for a base.
SELECT DISTINCT ON (quote_currency)
    quote_currency, rate, fetched_at
FROM fx_rates
WHERE base_currency = @base_currency
ORDER BY quote_currency, fetched_at DESC;

-- name: GetLatestFxFetchTime :one
-- Newest fetched_at for a source engine — the adapter throttle probe.
SELECT fetched_at
FROM fx_rates
WHERE source = @source
ORDER BY fetched_at DESC
LIMIT 1;

-- name: ListFxCurrencies :many
-- Picker list: active §58 currencies that have matrix coverage.
SELECT c.code, c.name, c.symbol
FROM currencies c
WHERE c.is_active = true
  AND EXISTS (SELECT 1 FROM fx_rates fr WHERE fr.quote_currency = c.code)
ORDER BY c.position ASC, c.code ASC;

-- name: PruneFxRates :execrows
-- Keep the most recently fetched rate per (pair, day); uuid has no ordering
-- so MIN(id) is invalid — DISTINCT ON picks the latest row per group.
DELETE FROM fx_rates
WHERE fx_rates.fetched_at < @cutoff_date
  AND fx_rates.id NOT IN (
      SELECT DISTINCT ON (base_currency, quote_currency, date_trunc('day', fetched_at))
             fx_rates.id
      FROM fx_rates
      WHERE fx_rates.fetched_at < @cutoff_date
      ORDER BY base_currency, quote_currency, date_trunc('day', fetched_at), fetched_at DESC
  );
