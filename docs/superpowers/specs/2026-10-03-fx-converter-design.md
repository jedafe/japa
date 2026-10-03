# FX Currency Converter — Mid-Market Rate Feed + Any-to-Any Converter

**Date:** 2026-10-03
**Status:** Design
**Builds on:** §74 (Remittance Rate Tracker), ADR-075 (hybrid provider model, `RateProvider` interface), §58 (`currencies` table — read-only reuse for picker labels), §03 (RBAC), §11 (Asynq), §35 (Notifications — via remittance alerts)
**Scope:** `internal/fx` (new), `internal/remittance`, `internal/server`, `internal/worker`, `internal/db/queries`, `migrations`, `apps/tools`, `apps/main`, `.jeos` (new §112, ADR-125, catalog updates)

## 1. Problem

JapaConnect's remittance tracker (§74) compares provider quotes per corridor, but there is no **mid-market FX rate** anywhere in the platform:

- Users cannot answer "what is 100 USD in NGN right now?" — the calculator only returns provider quotes (which embed markup + fees), and only for configured corridors.
- `SetProviders` is never called in `internal/server/server.go` — no live `RateProvider` adapter exists, so all rates today are admin-entered (`is_manual=true`).
- There is no benchmark against which to judge whether a provider's corridor rate is good.

Adding a mid-market feed carelessly would pollute the provider leaderboard — a no-fee interbank rate will almost always beat every remittance quote, making "best rate" misleading.

## 2. Goals

1. A standalone **any-to-any currency converter**: `GET /api/v1/fx/convert?amount=&from=&to=` computes cross-rates from a stored mid-market matrix. Any pair covered by the feed works without per-pair configuration.
2. The same feed **surfaced inside the remittance tracker as a labeled benchmark** — excluded from best-rate ranking and fee math, shown as a reference line.
3. One upstream API call per fetch tick serves both features — no duplicated fetch, no new fetch task.
4. Everything rides existing infrastructure: `RateProvider` interface, `remittance:fetch_rates` sweep, Valkey corridor caching, append-only rate history, feature-flag gating, encrypted `api_config`.

## 3. Non-goals

- No changes to §58 `currencies` credit-economy rates (fixed admin-set rates stay fixed; this feature reads the table only for currency names/symbols in pickers).
- No new frontend for rate alerts — alerts on mid-market corridors work automatically once benchmark rows exist in `remittance_rates`.
- No mobile app work (follow-up).
- No live trading-grade freshness — free-tier hourly data is the accepted floor.

## 4. Data source

**Open Exchange Rates** (`openexchangerates.org`) is the default engine. Free tier: `GET /latest.json?app_id={key}` returns a USD-base matrix covering 170+ currencies including NGN, updating hourly, ~1,000 requests/month quota. Two consequences baked into the design:

- **Per-provider throttle** — `api_config.min_interval_minutes` (default 60) makes the adapter no-op when invoked inside the interval, so the 30-minute `remittance:fetch_rates` sweep cannot burn the monthly quota on unchanged data.
- **Attribution** — the free tier requires attribution. `RateSnapshot.SourceURL` = `https://openexchangerates.org`; the frontend renders a "Powered by Open Exchange Rates" label.

The adapter is engine-pluggable: `api_config.engine` selects the upstream (`oxr` implemented now). A future source is a new engine branch, not a new module.

## 5. Data model — migration `000777_fx_rates`

```sql
CREATE TABLE fx_rates (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  base_currency  text NOT NULL,            -- 'USD' on OXR free tier
  quote_currency text NOT NULL,            -- ISO 4217
  rate           numeric(18,6) NOT NULL,   -- quote units per 1 base
  source         text NOT NULL DEFAULT 'oxr',
  fetched_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fx_rates_latest
  ON fx_rates (base_currency, quote_currency, fetched_at DESC);

ALTER TABLE remittance_providers
  ADD COLUMN is_benchmark boolean NOT NULL DEFAULT false;
```

Seeded provider row (same migration):

```sql
INSERT INTO remittance_providers
  (name, slug, website_url, data_source, api_config, is_active, is_benchmark, display_order)
VALUES (
  'Mid-market (Open Exchange Rates)', 'mid-market', 'https://openexchangerates.org',
  'api',
  -- encrypted via crypto.EncryptJSON at seed time is not possible in raw SQL;
  -- the row ships with is_active=false and NULL api_config; admin enters
  -- {engine, app_id, base_currency, min_interval_minutes, corridors} via the
  -- existing admin provider PATCH route, which encrypts on write.
  NULL,
  false, true, 0
);
```

`api_config` shape (entered by admin, encrypted at rest per ADR-075):

```json
{
  "engine": "oxr",
  "app_id": "<OXR App ID>",
  "base_currency": "USD",
  "min_interval_minutes": 60,
  "endpoint": "https://openexchangerates.org/api/latest.json",
  "corridors": ["GBP->NGN", "USD->NGN", "CAD->NGN", "EUR->NGN", "GBP->GHS", "USD->GHS"]
}
```

`endpoint` is optional — each engine carries a compiled-in default URL; the field exists so a future self-hosted/proxy feed or plan upgrade (e.g. paid-tier `base=` override) needs config only, not a code change.

`corridors` solves the bootstrap problem: `GetCorridors` derives pairs `SELECT DISTINCT send, receive FROM remittance_rates`, which is empty on a fresh install — the config list is the seed set the first fetch writes.

## 6. Backend — new package `internal/fx`

### 6.1 Service

```go
type Matrix struct {
    Base      string             // e.g. "USD"
    Rates     map[string]float64 // quote -> rate per 1 base (includes Base->1.0)
    FetchedAt time.Time
}

func (s *Service) GetMatrix(ctx context.Context, base string) (*Matrix, error)
func (s *Service) Convert(ctx context.Context, amount float64, from, to string) (*ConvertResult, error)
```

- `GetMatrix`: Valkey `jc:fx:matrix:{base}` (TTL `fx.cache_ttl_minutes`, default 120) → on miss, `DISTINCT ON (quote_currency) ... ORDER BY fetched_at DESC` over `fx_rates`, then cache.
- `Convert`: `rate = matrix[to] / matrix[from]`; result `{amount, from, to, rate, converted, fetched_at, stale}` where `stale` = matrix age > `fx.stale_hours` (default 48).
- Both honor `fx.enabled` via the injected `FeatureChecker`.

### 6.2 Adapter — implements `remittance.RateProvider`

`FetchRates(ctx, apiConfig, corridors)`:

1. Decrypt-parse config; throttle: if the newest `fx_rates` row for this `source` is younger than `min_interval_minutes`, return `nil, nil` (no-op — sweep stays cheap).
2. `GET {engine endpoint}` → parse base matrix.
3. Write the full matrix (including `base→base = 1.0`) to `fx_rates`; delete `jc:fx:matrix:*` for the base.
4. Emit `RateSnapshot`s for `config.corridors ∪ requested corridors` ∩ matrix coverage, `rate = m[to]/m[from]`, `source_url` = attribution URL. The remittance service's existing insert loop persists them as `remittance_rates` rows under `mid-market` — history, corridor discovery, cache invalidation, and rate alerts all work unchanged.

Registration in `server.go`:

```go
fxService := fx.NewService(fx.NewStore(database), settingsService)
fxService.SetCache(vkClient.Redis())
fxService.SetFeatureChecker(featureChecker)
remittanceService.SetProviders(map[string]remittance.RateProvider{
    "mid-market": fx.NewAdapter(fxService /* + http client */),
})
```

### 6.3 Routes (all public, behind `features.Gate(featureChecker, "fx.enabled")`)

| Route | Purpose |
|---|---|
| `GET /api/v1/fx/convert?amount=&from=&to=` | any-to-any conversion + `{rate, converted, fetched_at, stale}` |
| `GET /api/v1/fx/rates?base=USD` | matrix for a base, cross-computed if base ≠ stored base |
| `GET /api/v1/fx/currencies` | picker data: `currencies` ∩ matrix coverage → `{code, name, symbol}[]` |

## 7. Remittance changes (benchmark discipline)

- `RateInfo` and `ProviderInfo` gain `is_benchmark` (from the new column).
- `GetBestRate` and `Calculate` **exclude** benchmark rows from ranking. `CalculateResult` gains an optional `benchmark { rate, receive_amount }` so the UI can show "mid-market would yield ₦X" above ranked provider results.
- `remittance_providers.is_benchmark` is surfaced in the admin provider form (checkbox) and list badge.

## 8. Worker

- **No new fetch task** — the adapter rides `remittance:fetch_rates` (per-provider sweep).
- **New task** `fx:prune_rates` (low queue, weekly): deletes `fx_rates` rows older than `fx.history_retention_days` (default 90 — a full matrix is ~165 rows per fetch, heavier than provider rows). Registered in the §11 task catalog before use (Rule 7).

## 9. Config & catalogs

| Type | Key | Default |
|---|---|---|
| Feature flag | `fx.enabled` | `true` |
| system_settings | `fx.cache_ttl_minutes` | `120` |
| system_settings | `fx.stale_hours` | `48` |
| system_settings | `fx.history_retention_days` | `90` |
| system_settings | `fx.default_base_currency` | `USD` |
| Valkey | `jc:fx:matrix:{base}` | TTL per `fx.cache_ttl_minutes` |

No new `JC_*` env vars — the API key lives in encrypted `api_config`. No new RBAC nodes — reads are public; provider configuration rides `remittance.manage`/`remittance.fetch`.

## 10. Frontend

**`apps/tools`** — registry entry `currency-converter` → `/currency-converter`:
- Amount input; from/to selects populated from `/api/v1/fx/currencies`; swap button.
- Result card: converted amount, unit rate, inverse rate, `fetched_at`, stale warning, OXR attribution link.
- i18n keys `tool.fx_converter.*` in en + fr locale files.

**`apps/main`** — `(app)/tools/currency-converter` page in the main app shell, same API, following the existing `(app)/tools/remittance` page pattern.

**Remittance pages (tools + main)** — benchmark rows render a "Mid-market" badge via `is_benchmark`, never win the best-rate highlight; the calculator response renders `benchmark` as a reference line above provider results.

## 11. JEOS updates (before code — AGENTS.md)

- New `sections/112-fx-converter.md`, `adr/ADR-125-fx-converter.md`; `JEOS.md` index entries.
- Catalogs: `api-routes-catalog` (+3), `valkey-keys-catalog` (+1), `asynq-tasks` (+1 `fx:prune_rates`), `system-settings-catalog` (+4), feature-flag seed.
- §74 amendment note: benchmark providers exist, `is_benchmark` semantics, exclusion from ranking.

## 12. Error handling

- OXR failure → `FetchRates` returns error → Asynq retries per §11 policy; converter keeps serving the last matrix with `stale: true`.
- Empty matrix (never fetched) → `503 fx_unavailable`.
- `amount <= 0` → `400 invalid_amount`; currency not in matrix → `400 unsupported_currency`.
- Cache failures degrade to DB reads (same graceful-degradation pattern as §74).

## 13. Testing & verification

- `internal/fx`: cross-rate math (identity pair, base=from, base=to, cross pair), throttle boundary, adapter with stubbed HTTP server (fixture `latest.json`), stale flag, `Convert` on empty matrix.
- `internal/remittance`: benchmark excluded from `Calculate` ranking and `GetBestRate`; `benchmark` field populated.
- Handlers: param validation on all three routes; flag-disabled → gated.
- Commands: `make sqlc-gen`, `go build ./...`, `go test ./internal/fx ./internal/remittance`, `bun run check` in `apps/tools` and `apps/main`.

## 14. Out of scope / follow-ups

- Mobile app converter screen.
- Provider-side real adapters (Wise/Remitly) — same `RateProvider` contract, separate effort.
- Historical mid-market chart UI (data accrues automatically in `fx_rates`; chart endpoint is a cheap follow-up if wanted).
