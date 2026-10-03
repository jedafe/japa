# FX Currency Converter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a mid-market FX rate feed (Open Exchange Rates) that powers both a standalone any-to-any currency converter (`/api/v1/fx/*` + converter tool pages) and a benchmark provider inside the existing remittance tracker — per `docs/superpowers/specs/2026-10-03-fx-converter-design.md`.

**Architecture:** New `internal/fx` package owns an append-only `fx_rates` base-currency matrix and exposes convert/rates/currencies endpoints. Its `Adapter` implements `remittance.RateProvider`, so one upstream call per `remittance:fetch_rates` tick writes the matrix AND emits corridor snapshots the existing remittance service persists under a seeded `mid-market` provider. Benchmark rows are excluded from best-rate/calculator ranking via a new `remittance_providers.is_benchmark` flag.

**Tech Stack:** Go (chi, sqlc/pgx, Asynq, Valkey), SvelteKit 2 + Svelte 5 runes (apps/tools, apps/main, apps/admin), PostgreSQL migrations, `.jeos` spec submodule.

## Global Constraints

- **JEOS before code** — `.jeos` (a git submodule) gets its section/ADR/catalog updates committed BEFORE backend code lands (AGENTS.md).
- **Rule 2** — never hand-edit `internal/db/gen/*`; edit `internal/db/queries/*.sql` then `make sqlc-gen`.
- **Rule 3** — no hardcoded config; values come from `system_settings` (via `config.Settings` getters) or `JC_*` env.
- **Rule 4** — all Valkey keys use `valkey.MustKey` (`jc:` prefix); new patterns go in `.jeos/conventions/valkey-keys-catalog.md` first.
- **Rule 5** — errors via `errpkg.WriteError(w, status, "snake_case_code", "message")`; success via `errpkg.WriteJSON(w, status, map[string]any{"data": ...})`.
- **Rule 7** — new Asynq task names must exist in `.jeos/conventions/asynq-tasks.md` before code uses them.
- **Rule 16** — no hardcoded data in frontend; currencies/options come from the API.
- **Rule 17** — no AI attribution anywhere (commits, comments, docs).
- **Rule 18** — JS/TS tooling via `bun`/`bunx`/`bun run` only.
- Feature flags: any `features.Gate`/`IsEnabled` key MUST be in `internal/features/registry.go` (`features_test.go` enforces).
- Go numeric columns use `pgtype.Numeric` with `numericFromFloat`/`numericToFloat64` helpers.
- Migration naming: `migrations/NNNNNN_<name>.up.sql` + `.down.sql`; next free number is **000777**.
- Spec values: `fx.cache_ttl_minutes=120`, `fx.stale_hours=48`, `fx.history_retention_days=90`, `fx.default_base_currency='USD'`, `fx.subdomain='currency-converter'`, `fx.custom_domain=''`; task `fx:prune_rates` low queue weekly; Valkey `jc:fx:matrix:{base}` TTL per `fx.cache_ttl_minutes`; OXR attribution URL `https://openexchangerates.org`.

---

### Task 1: JEOS spec files + catalog entries (.jeos submodule)

`.jeos` is a git submodule (`github.com/jedafe/japaconnect-engineering`). Commit inside `.jeos/` on `main`.

**Files:**
- Create: `.jeos/sections/112-fx-converter.md`
- Create: `.jeos/adr/ADR-125-fx-converter.md`
- Modify: `.jeos/JEOS.md` (index entries for §112 + ADR-125)
- Modify: `.jeos/conventions/feature-flag-naming.md` (+1 row)
- Modify: `.jeos/conventions/system-settings-catalog.md` (+6 rows)
- Modify: `.jeos/conventions/valkey-keys-catalog.md` (+1 row)
- Modify: `.jeos/conventions/asynq-tasks.md` (+2 rows — one in the task table, one in the schedule table)
- Modify: `.jeos/conventions/api-routes-catalog.md` (+3 routes, next to the remittance block ~line 1954)
- Modify: `.jeos/sections/74-remittance-tracker.md` (short amendment note re benchmark providers)

**Interfaces:**
- Produces: the JEOS identifiers every later task references — section `§112`, ADR `ADR-125`, flag `fx.enabled`, settings `fx.*`, task `fx:prune_rates`, Valkey `jc:fx:matrix:*`, routes `/api/v1/fx/*`.

- [ ] **Step 1: Write `.jeos/sections/112-fx-converter.md`**

Copy the spec sections verbatim from `docs/superpowers/specs/2026-10-03-fx-converter-design.md` (sections 1–14), re-titled `# Section 110 — FX Currency Converter`, prefixed with the standard section header style used in `sections/74-remittance-tracker.md` (read that file's first ~30 lines and mirror its front matter/heading conventions).

- [ ] **Step 2: Write `.jeos/adr/ADR-125-fx-converter.md`**

```markdown
# ADR-125 — FX Currency Converter (Mid-Market Feed + Any-to-Any Converter)

**Status:** Accepted
**Date:** 2026-10-03
**Drivers:** Users need a plain "what is X in Y" mid-market converter and the
remittance tracker needs a benchmark rate to judge provider quotes against.
The §74 `RateProvider` interface was designed for pluggable rate adapters but
none was ever registered.
**References:** Section 74 + ADR-075 (provider model, corridor plumbing),
Section 58 (currencies — read-only for picker labels), Section 11 (Asynq),
Section 42 (feature flags), ADR-086 (tools multi-surface host detection).

## Decision

1. **New `fx_rates` table** stores a base→quote mid-market matrix per fetch
   (append-only; ~165 rows/fetch; pruned weekly at 90-day retention).
2. **`internal/fx` package** serves `GET /api/v1/fx/convert`, `/fx/rates`,
   `/fx/currencies` (public, `fx.enabled`-gated).
3. **`fx.Adapter` implements `remittance.RateProvider`** and is registered
   under the `mid-market` provider slug — one upstream call per
   `remittance:fetch_rates` tick writes the matrix AND returns corridor
   snapshots. No new fetch task.
4. **`remittance_providers.is_benchmark`** marks the mid-market provider;
   benchmark rows are excluded from best-rate/calculator ranking and rendered
   as a labeled reference.
5. **Engine-pluggable api_config** `{engine, app_id, base_currency,
   min_interval_minutes, endpoint, corridors}` — OXR is the first engine;
   `min_interval_minutes` throttles calls inside the provider's fetch tick.
6. **Converter tool on both surfaces** — `apps/tools` registry + host-detect
   entries, `apps/main` tools page.

## Consequences

Positive: zero new fetch tasks, corridor history/alerts work for mid-market
corridors for free, benchmark discipline keeps provider rankings honest.
Negative/accepted: free-tier FX APIs are USD-base and hourly — cross-rate
math and staleness flag absorb both; OXR free tier requires attribution
(rendered in the UI).
```

- [ ] **Step 3: Update the catalogs**

`.jeos/conventions/feature-flag-naming.md` — add row to the same table that holds `remittance.enabled` (~line 13):

```markdown
| `fx.enabled` | `true` | §112 |
```

`.jeos/conventions/system-settings-catalog.md` — add a `## FX Converter (§112)` table next to the remittance one (~line 529):

```markdown
| `fx.cache_ttl_minutes` | `120` | int | Valkey TTL for the mid-market matrix cache |
| `fx.stale_hours` | `48` | int | Matrix age before convert/rates responses flag `stale:true` |
| `fx.history_retention_days` | `90` | int | Days of fx_rates history to retain |
| `fx.default_base_currency` | `USD` | string | Base currency the converter matrix is stored under |
| `fx.subdomain` | `currency-converter` | string | Standalone subdomain (ADR-086) |
| `fx.custom_domain` | `` | string | Optional custom domain (ADR-086) |
```

`.jeos/conventions/valkey-keys-catalog.md` — add next to the `jc:remit:*` rows (~line 238):

```markdown
| `jc:fx:matrix:{base}` | Latest mid-market FX matrix for a base currency | fx.cache_ttl_minutes (default 120 min) | §112 |
```

`.jeos/conventions/asynq-tasks.md` — add to the task table AND the schedule table (mirror the `remittance:prune_old_rates` rows ~lines 150/168):

```markdown
| `fx:prune_rates` | Weekly — delete fx_rates matrix rows past fx.history_retention_days (§112) | Yes (sweep) |
```

```markdown
| `fx:prune_rates` | Weekly | FX matrix retention pruning |
```

`.jeos/conventions/api-routes-catalog.md` — add directly after the remittance route block (~line 1967):

```markdown
GET    /api/v1/fx/convert                           — ?amount=&from=&to= — mid-market conversion (§112)
GET    /api/v1/fx/rates                             — ?base=USD — latest mid-market matrix (§112)
GET    /api/v1/fx/currencies                        — picker list: currencies ∩ matrix coverage (§112)
```

`.jeos/JEOS.md` — add §112 + ADR-125 index entries wherever the §74/ADR-075 entries live (search for `74-remittance` and `ADR-075` in that file and mirror the format).

`.jeos/sections/74-remittance-tracker.md` — append:

```markdown
### Amendment (2026-10-03): benchmark providers

`remittance_providers.is_benchmark` (added by migration 000777, §112/ADR-125)
marks a provider whose rates are a mid-market reference, not a sellable
quote. Benchmark providers are excluded from `GetBestRate` and the
`Calculate` ranking and are exposed separately as `CalculateResult.benchmark`.
The seeded `mid-market` provider is fed by the `internal/fx` OXR adapter via
the standard `RateProvider` contract.
```

- [ ] **Step 4: Commit inside the submodule**

```bash
cd .jeos
git add -A
git commit -m "docs: §112 FX currency converter + ADR-125 + catalog entries (fx flag/settings/keys/tasks/routes)"
cd ..
```

---

### Task 2: Migration 000777 + sqlc queries + regen

**Files:**
- Create: `migrations/000777_fx_rates.up.sql`
- Create: `migrations/000777_fx_rates.down.sql`
- Create: `internal/db/queries/fx.sql`
- Modify: `internal/db/queries/remittance.sql` (provider + corridor-rate queries)
- Modify: `internal/control/settings_catalog.go` (`fx.*` catalog entries — reviewed: every V2 setting key is registered here; missing entries show up untyped/unclassified in the Control Panel and `IsPublicSetting` returns false, which would silently break `fx.subdomain` host detection on any admin upsert)
- Generated (do NOT hand-edit): `internal/db/gen/fx.sql.go`, `internal/db/gen/remittance.sql.go`, `internal/db/gen/models.go`

**Interfaces:**
- Produces (sqlc-generated names later tasks rely on):
  - `gen.FxRate` model, `gen.InsertFxRateParams`, `gen.InsertFxRate`, `gen.GetLatestFxMatrixParams/Row`, `gen.GetLatestFxFetchTime`, `gen.PruneFxRates`, `gen.ListFxCurrenciesRow`
  - `gen.RemittanceProvider.IsBenchmark`, `gen.GetLatestRatesForCorridorRow.ProviderIsBenchmark`, `gen.CreateRemittanceProviderParams.IsBenchmark`, `gen.UpdateRemittanceProviderParams.IsBenchmark`

- [ ] **Step 1: Write the up migration**

`migrations/000777_fx_rates.up.sql` (match the `BEGIN;...COMMIT;` wrapping style of 000776):

```sql
-- 000777_fx_rates.up.sql
-- §112 / ADR-125: mid-market FX matrix + remittance benchmark provider flag.
BEGIN;

CREATE TABLE fx_rates (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    base_currency  text NOT NULL,
    quote_currency text NOT NULL,
    rate           numeric(18,6) NOT NULL,
    source         text NOT NULL DEFAULT 'oxr',
    fetched_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fx_rates_latest
    ON fx_rates (base_currency, quote_currency, fetched_at DESC);

ALTER TABLE remittance_providers
    ADD COLUMN is_benchmark boolean NOT NULL DEFAULT false;

-- Seeded benchmark provider. Inactive until an admin pastes an OXR App ID
-- into api_config via PATCH /api/v1/admin/remittance/providers/:id — the
-- handler encrypts api_config on write (ADR-075 §1).
-- remittance_providers_slug_key is a UNIQUE index — conflict-safe insert.
INSERT INTO remittance_providers
    (name, slug, website_url, data_source, api_config, is_active, is_benchmark, display_order)
VALUES
    ('Mid-market (Open Exchange Rates)', 'mid-market', 'https://openexchangerates.org',
     'api', NULL, false, true, 0)
ON CONFLICT (slug) DO NOTHING;

-- §112 system_settings (catalog: conventions/system-settings-catalog.md).
INSERT INTO system_settings (key, value, is_public) VALUES
    ('fx.cache_ttl_minutes', '120', false),
    ('fx.stale_hours', '48', false),
    ('fx.history_retention_days', '90', false),
    ('fx.default_base_currency', '"USD"', false),
    ('fx.subdomain', '"currency-converter"', true),
    ('fx.custom_domain', '""', true)
ON CONFLICT (key) DO NOTHING;

COMMIT;
```

Note: `system_settings.value` is `jsonb` — numbers are bare JSON, strings are double-quoted. **Verified:** `GetPublicSettings` (`internal/db/gen/settings.sql.go`) filters `WHERE is_public = true`; `remittance.subdomain` is seeded `is_public=false` and resolves only via the hardcoded `DEFAULT_SUBDOMAINS` fallback in `apps/tools/src/hooks.server.ts`. We seed `fx.subdomain`/`fx.custom_domain` with `is_public=true` so admin-set custom domains actually reach `/site/settings/public`, AND mark them `Public: true` in `internal/control/settings_catalog.go` (next step) so admin upserts don't silently flip the flag back to false.

- [ ] **Step 2: Write the down migration**

`migrations/000777_fx_rates.down.sql`:

```sql
-- 000777_fx_rates.down.sql
BEGIN;
DELETE FROM system_settings WHERE key IN (
    'fx.cache_ttl_minutes', 'fx.stale_hours', 'fx.history_retention_days',
    'fx.default_base_currency', 'fx.subdomain', 'fx.custom_domain');
DELETE FROM remittance_providers WHERE slug = 'mid-market' AND is_benchmark = true;
ALTER TABLE remittance_providers DROP COLUMN IF EXISTS is_benchmark;
DROP TABLE IF EXISTS fx_rates;
COMMIT;
```

- [ ] **Step 3: Write `internal/db/queries/fx.sql`**

```sql
-- fx.sql — FX Currency Converter queries (Section 110, ADR-125).
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

-- name: PruneFxRates :exec
DELETE FROM fx_rates
WHERE fx_rates.fetched_at < @cutoff_date
  AND fx_rates.id NOT IN (
      SELECT MIN(fx_rates.id) FROM fx_rates
      WHERE fx_rates.fetched_at < @cutoff_date
      GROUP BY base_currency, quote_currency, date_trunc('day', fetched_at)
  );
```

- [ ] **Step 4: Update `internal/db/queries/remittance.sql` for `is_benchmark`**

Three edits:

a) `CreateRemittanceProvider` — add `is_benchmark` to columns/params:

```sql
INSERT INTO remittance_providers (name, slug, logo_media_id, website_url, data_source, api_config, is_active, is_benchmark, display_order) VALUES (@name, @slug, sqlc.narg(logo_media_id), sqlc.narg(website_url), @data_source, sqlc.narg(api_config), @is_active, sqlc.narg(is_benchmark), @display_order)
```

b) `UpdateRemittanceProvider` — add to the SET list:

```sql
 is_benchmark = COALESCE(sqlc.narg(is_benchmark), is_benchmark),
```

c) `GetLatestRatesForCorridor` — add `rp.is_benchmark AS provider_is_benchmark` to the SELECT list (after `rp.website_url AS provider_website`).

- [ ] **Step 5: Register `fx.*` keys in the Control settings catalog**

In `internal/control/settings_catalog.go`, add a §112 block next to the other V2 entries (after the `remittance.*` block ~line 484), matching the existing entry shape `{Type, DefaultValue, Description, Panel, Public, Min/Max}`:

```go
// FX Currency Converter (§112, ADR-125)
"fx.enabled":                {Type: "bool", DefaultValue: "true", Description: "Global FX converter toggle.", Panel: "control"},
"fx.cache_ttl_minutes":      {Type: "int", DefaultValue: "120", Min: 5, Description: "Valkey TTL for the mid-market rate matrix (minutes).", Panel: "control"},
"fx.stale_hours":            {Type: "int", DefaultValue: "48", Min: 1, Description: "Hours before a served FX matrix is flagged stale.", Panel: "control"},
"fx.history_retention_days": {Type: "int", DefaultValue: "90", Min: 7, Description: "Days of fx_rates matrix history to retain.", Panel: "control"},
"fx.default_base_currency":  {Type: "string", DefaultValue: "USD", Description: "Default base currency for /fx/rates.", Panel: "control"},
"fx.subdomain":              {Type: "string", DefaultValue: "currency-converter", Public: true, Description: "Standalone subdomain (ADR-086).", Panel: "control"},
"fx.custom_domain":          {Type: "string", DefaultValue: "", Public: true, Description: "Optional custom domain (ADR-086).", Panel: "control"},
```

`Public: true` on the two host keys is load-bearing: `IsPublicSetting` feeds `is_public` on admin upserts, and `apps/tools` host detection reads only public settings.

- [ ] **Step 6: Apply migration + regenerate sqlc**

```bash
make migrate    # runs `go run ./cmd/migrate/` — applies pending migrations incl. 000777
make sqlc-gen
go build ./internal/db/gen/
```

Expected: build passes; `gen.RemittanceProvider` has `IsBenchmark bool`; `gen.GetLatestRatesForCorridorRow` has `ProviderIsBenchmark bool`.

- [ ] **Step 7: Commit**

```bash
git add migrations/000777_fx_rates.*.sql internal/db/queries/fx.sql internal/db/queries/remittance.sql internal/db/gen/ internal/control/settings_catalog.go
git commit -m "feat(db): fx_rates matrix + remittance_providers.is_benchmark + fx settings (§112, ADR-125)"
```

---

### Task 3: `internal/fx` — store + service (matrix read, convert)

**Files:**
- Create: `internal/fx/store.go`
- Create: `internal/fx/service.go`
- Test: `internal/fx/service_test.go`

**Interfaces:**
- Consumes: `gen.InsertFxRateParams`, `gen.GetLatestFxMatrixRow`, `gen.GetLatestFxFetchTime`, `gen.PruneFxRates`, `gen.ListFxCurrenciesRow` (Task 2); `config.Settings.GetIntOr/GetStringOr`, `valkey.MustKey`, `valkeycompat.Cmdable` (existing patterns in `internal/remittance/service.go`).
- Produces:
  - `fx.NewStore(database *db.DB) *Store`
  - `fx.NewService(store *Store, settings *config.Settings) *Service`
  - `(*Service).SetCache(c valkeycompat.Cmdable)`, `.SetFeatureChecker(c FeatureChecker)`
  - `type Matrix struct { Base string; Rates map[string]float64; FetchedAt time.Time }`
  - `(*Service).GetMatrix(ctx, base string) (*Matrix, error)`
  - `type ConvertResult struct { Amount, From, To, Rate, Converted float64 /* + json tags */; FetchedAt string; Stale bool }` — exact fields below
  - `(*Service).Convert(ctx, amount float64, from, to string) (*ConvertResult, error)`
  - `(*Service).ListCurrencies(ctx) ([]gen.ListFxCurrenciesRow, error)`
  - `(*Service).LastFetchTime(ctx, source string) (time.Time, error)`
  - `(*Service).PruneRates(ctx) (int, error)` (worker Task 7)
  - `(*Service).InsertMatrix(ctx, base, source string, rates map[string]float64) error` (adapter Task 4)
  - `type FeatureChecker interface { IsEnabled(ctx context.Context, key string) bool }` (same shape as remittance's)

- [ ] **Step 1: Write the failing test for Convert math**

`internal/fx/service_test.go` — pure-math test via an unexported helper so no DB is needed:

```go
package fx

import (
	"testing"
	"time"
)

func TestComputeConvert_CrossRate(t *testing.T) {
	m := &Matrix{
		Base:      "USD",
		Rates:     map[string]float64{"USD": 1, "NGN": 1600, "GBP": 0.8, "EUR": 0.9},
		FetchedAt: time.Now(),
	}

	tests := []struct {
		name         string
		amount       float64
		from, to     string
		wantRate     float64
		wantConv     float64
		wantErr      bool
	}{
		{"identity", 100, "USD", "USD", 1, 100, false},
		{"base to quote", 100, "USD", "NGN", 1600, 160000, false},
		{"quote to base", 1600, "NGN", "USD", 1.0 / 1600.0, 1, false},
		{"cross pair", 100, "GBP", "NGN", 2000, 200000, false},
		{"cross pair inverse", 2000, "NGN", "GBP", 0.0005, 1, false},
		{"unsupported from", 1, "XXX", "NGN", 0, 0, true},
		{"unsupported to", 1, "USD", "XXX", 0, 0, true},
		{"zero amount", 0, "USD", "NGN", 0, 0, true},
		{"negative amount", -5, "USD", "NGN", 0, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rate, converted, err := computeConvert(m, tc.amount, tc.from, tc.to)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got rate=%v converted=%v", rate, converted)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := rate - tc.wantRate; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("rate: got %v want %v", rate, tc.wantRate)
			}
			if diff := converted - tc.wantConv; diff > 1e-6 || diff < -1e-6 {
				t.Fatalf("converted: got %v want %v", converted, tc.wantConv)
			}
		})
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

```bash
go test ./internal/fx/ -run TestComputeConvert_CrossRate -v
```

Expected: FAIL — package/function undefined.

- [ ] **Step 3: Write `internal/fx/store.go`**

Mirror `internal/remittance/store.go` structure (thin wrapper, no business logic):

```go
// Package fx implements the FX Currency Converter (Section 110, ADR-125):
// a mid-market base→quote matrix fed by pluggable rate engines, powering the
// any-to-any converter and the remittance benchmark provider.
//
// Ref: japaconnect-engineering/sections/112-fx-converter.md
//
//	japaconnect-engineering/adr/ADR-125-fx-converter.md
package fx

import (
	"context"
	"time"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for fx operations.
// It is the data-access layer — no business logic here.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates an fx store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// InsertRate inserts a single matrix cell row.
func (s *Store) InsertRate(ctx context.Context, arg gen.InsertFxRateParams) (gen.FxRate, error) {
	return s.queries.InsertFxRate(ctx, arg)
}

// GetLatestMatrix returns the latest rate per quote for a base currency.
func (s *Store) GetLatestMatrix(ctx context.Context, base string) ([]gen.GetLatestFxMatrixRow, error) {
	return s.queries.GetLatestFxMatrix(ctx, base)
}

// LastFetchTime returns the newest fetched_at for a source engine.
func (s *Store) LastFetchTime(ctx context.Context, source string) (time.Time, error) {
	t, err := s.queries.GetLatestFxFetchTime(ctx, source)
	if err != nil {
		return time.Time{}, err
	}
	return t.Time, nil
}

// ListCurrencies returns active §58 currencies that have matrix coverage.
func (s *Store) ListCurrencies(ctx context.Context) ([]gen.ListFxCurrenciesRow, error) {
	return s.queries.ListFxCurrencies(ctx)
}

// PruneRates deletes rows older than cutoff, keeping one snapshot per
// base/quote/day for older data (same downsampling as remittance §74).
func (s *Store) PruneRates(ctx context.Context, cutoff time.Time) error {
	return s.queries.PruneFxRates(ctx,
		pgtype.Timestamptz{Time: cutoff, Valid: true},
	)
}
```

(Add `"github.com/jackc/pgx/v5/pgtype"` import.)

- [ ] **Step 4: Write `internal/fx/service.go`**

Follow `internal/remittance/service.go` conventions (FeatureChecker, settings getters, numeric helpers duplicated package-locally since they're unexported there — copy `numericFromFloat`/`numericToFloat64`/`rfc3339` into this file or a `internal/fx/helpers.go`):

```go
package fx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
)

// ErrUnsupportedCurrency is returned when a currency code is absent from
// the latest matrix. Handlers map it to 400 unsupported_currency.
var ErrUnsupportedCurrency = errors.New("fx: unsupported currency")

// ErrEmptyMatrix is returned when no matrix has been fetched yet.
// Handlers map it to 503 fx_unavailable.
var ErrEmptyMatrix = errors.New("fx: no rate matrix available")

// FeatureChecker resolves feature flag state (same contract as remittance).
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Service holds the FX converter business logic.
type Service struct {
	store    *Store
	settings *config.Settings
	cache    valkeycompat.Cmdable // nil = caching disabled
	checker  FeatureChecker
}

func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

func (s *Service) SetCache(c valkeycompat.Cmdable)       { s.cache = c }
func (s *Service) SetFeatureChecker(c FeatureChecker)     { s.checker = c }

func (s *Service) IsEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "fx.enabled")
	}
	if s.settings == nil {
		return true
	}
	return s.settings.GetBoolOr(ctx, "fx.enabled", true)
}

func (s *Service) cacheTTL(ctx context.Context) time.Duration {
	if s.settings == nil {
		return 120 * time.Minute
	}
	return time.Duration(s.settings.GetIntOr(ctx, "fx.cache_ttl_minutes", 120)) * time.Minute
}

func (s *Service) staleHours(ctx context.Context) int {
	if s.settings == nil {
		return 48
	}
	return s.settings.GetIntOr(ctx, "fx.stale_hours", 48)
}

// HistoryRetentionDays returns fx.history_retention_days (default 90).
func (s *Service) HistoryRetentionDays(ctx context.Context) int {
	if s.settings == nil {
		return 90
	}
	return s.settings.GetIntOr(ctx, "fx.history_retention_days", 90)
}

// DefaultBaseCurrency returns fx.default_base_currency (default USD).
func (s *Service) DefaultBaseCurrency(ctx context.Context) string {
	if s.settings == nil {
		return "USD"
	}
	return s.settings.GetStringOr(ctx, "fx.default_base_currency", "USD")
}

// Matrix is the latest mid-market rate set for a base currency.
type Matrix struct {
	Base      string             `json:"base"`
	Rates     map[string]float64 `json:"rates"`
	FetchedAt time.Time          `json:"fetched_at"`
}

func matrixCacheKey(base string) string {
	return valkey.MustKey("fx:matrix::%s", base)
}

// GetMatrix returns the latest matrix for a base: Valkey first, DB fallback.
func (s *Service) GetMatrix(ctx context.Context, base string) (*Matrix, error) {
	key := matrixCacheKey(base)
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, key).Result(); err == nil && cached != "" {
			var m Matrix
			if err := json.Unmarshal([]byte(cached), &m); err == nil {
				return &m, nil
			}
		}
	}

	rows, err := s.store.GetLatestMatrix(ctx, base)
	if err != nil {
		return nil, fmt.Errorf("fx: get matrix: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrEmptyMatrix
	}

	m := &Matrix{Base: base, Rates: make(map[string]float64, len(rows)+1)}
	m.Rates[base] = 1
	for _, r := range rows {
		m.Rates[r.QuoteCurrency] = numericToFloat64(r.Rate)
		if r.FetchedAt.Valid && r.FetchedAt.Time.After(m.FetchedAt) {
			m.FetchedAt = r.FetchedAt.Time
		}
	}

	if s.cache != nil {
		if data, err := json.Marshal(m); err == nil {
			if err := s.cache.Set(ctx, key, data, s.cacheTTL(ctx)).Err(); err != nil {
				slog.Warn("fx: matrix cache set failed", "error", err)
			}
		}
	}

	return m, nil
}

// InvalidateMatrix drops the cached matrix for a base (post-fetch).
func (s *Service) InvalidateMatrix(ctx context.Context, base string) {
	if s.cache == nil {
		return
	}
	if err := s.cache.Del(ctx, matrixCacheKey(base)).Err(); err != nil {
		slog.Warn("fx: invalidate matrix cache failed", "base", base, "error", err)
	}
}

// ConvertResult is the JSON-serialisable conversion output.
type ConvertResult struct {
	Amount    float64 `json:"amount"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	Rate      float64 `json:"rate"`
	Converted float64 `json:"converted"`
	FetchedAt string  `json:"fetched_at"`
	Stale     bool    `json:"stale"`
}

// computeConvert does the pure cross-rate math (unit-testable, no I/O).
func computeConvert(m *Matrix, amount float64, from, to string) (rate, converted float64, err error) {
	if amount <= 0 {
		return 0, 0, errors.New("fx: amount must be positive")
	}
	fromRate, ok := m.Rates[from]
	if !ok {
		return 0, 0, fmt.Errorf("%w: %s", ErrUnsupportedCurrency, from)
	}
	toRate, ok := m.Rates[to]
	if !ok {
		return 0, 0, fmt.Errorf("%w: %s", ErrUnsupportedCurrency, to)
	}
	rate = toRate / fromRate
	return rate, rate * amount, nil
}

// Convert converts amount of `from` to `to` against the latest matrix.
// `from`/`to` are normalised to uppercase ISO codes by the caller (handler).
func (s *Service) Convert(ctx context.Context, amount float64, from, to string) (*ConvertResult, error) {
	if !s.IsEnabled(ctx) {
		return nil, errors.New("fx: converter disabled")
	}

	m, err := s.GetMatrix(ctx, s.DefaultBaseCurrency(ctx))
	if err != nil {
		return nil, err
	}

	rate, converted, err := computeConvert(m, amount, from, to)
	if err != nil {
		return nil, err
	}

	return &ConvertResult{
		Amount:    amount,
		From:      from,
		To:        to,
		Rate:      rate,
		Converted: converted,
		FetchedAt: m.FetchedAt.UTC().Format(time.RFC3339),
		Stale:     time.Since(m.FetchedAt) > time.Duration(s.staleHours(ctx))*time.Hour,
	}, nil
}

// GetRates returns the matrix re-based to `base` (cross-computed when the
// stored base differs). Same payload shape as a Matrix plus `stale`.
func (s *Service) GetRates(ctx context.Context, base string) (*Matrix, bool, error) {
	if !s.IsEnabled(ctx) {
		return nil, false, errors.New("fx: converter disabled")
	}

	m, err := s.GetMatrix(ctx, s.DefaultBaseCurrency(ctx))
	if err != nil {
		return nil, false, err
	}

	if base == m.Base {
		stale := time.Since(m.FetchedAt) > time.Duration(s.staleHours(ctx))*time.Hour
		return m, stale, nil
	}

	pivot, ok := m.Rates[base]
	if !ok {
		return nil, false, fmt.Errorf("%w: %s", ErrUnsupportedCurrency, base)
	}
	rebased := &Matrix{Base: base, Rates: make(map[string]float64, len(m.Rates)), FetchedAt: m.FetchedAt}
	for q, r := range m.Rates {
		rebased.Rates[q] = r / pivot
	}
	stale := time.Since(m.FetchedAt) > time.Duration(s.staleHours(ctx))*time.Hour
	return rebased, stale, nil
}

// ListCurrencies returns the picker currency list.
func (s *Service) ListCurrencies(ctx context.Context) ([]gen.ListFxCurrenciesRow, error) {
	if !s.IsEnabled(ctx) {
		return nil, errors.New("fx: converter disabled")
	}
	return s.store.ListCurrencies(ctx)
}

// LastFetchTime returns the newest fetch timestamp for a source engine —
// used by the adapter throttle. Returns zero time when nothing fetched yet.
func (s *Service) LastFetchTime(ctx context.Context, source string) (time.Time, error) {
	t, err := s.store.LastFetchTime(ctx, source)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("fx: last fetch time: %w", err)
	}
	return t, nil
}

// InsertMatrix persists a full base→quote matrix atomically. Rates map
// includes the base→base=1.0 cell (adapter adds it if absent).
func (s *Service) InsertMatrix(ctx context.Context, base, source string, rates map[string]float64) error {
	tx, err := s.store.pool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("fx: insert matrix: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := gen.New(tx)
	for quote, rate := range rates {
		if _, err := q.InsertFxRate(ctx, gen.InsertFxRateParams{
			BaseCurrency:  base,
			QuoteCurrency: quote,
			Rate:          numericFromFloat(rate),
			Source:        &source,
		}); err != nil {
			return fmt.Errorf("fx: insert matrix row %s: %w", quote, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("fx: insert matrix: commit: %w", err)
	}

	s.InvalidateMatrix(ctx, base)
	return nil
}

// PruneRates deletes matrix rows past retention (fx:prune_rates task, §112).
func (s *Service) PruneRates(ctx context.Context) (int, error) {
	cutoff := time.Now().AddDate(0, 0, -s.HistoryRetentionDays(ctx))
	if err := s.store.PruneRates(ctx, cutoff); err != nil {
		return 0, fmt.Errorf("fx: prune rates: %w", err)
	}
	return 1, nil
}
```

(Add imports `"github.com/jackc/pgx/v5"`, `"github.com/jackc/pgx/v5/pgtype"`. Copy `numericFromFloat`/`numericToFloat64` helpers from `internal/remittance/service.go` lines 226–246 into this file — they are unexported there.)

NOTE on `s.store.pool`: `Store.pool` is the `*db.DB` — mark it used (the remittance store does the same for tx paths). If the field ends up unused by `InsertMatrix`'s final form, drop the field; prefer keeping it since tx access is required.

- [ ] **Step 5: Run tests — verify pass**

```bash
go test ./internal/fx/ -v
go vet ./internal/fx/
```

- [ ] **Step 6: Commit**

```bash
git add internal/fx/
git commit -m "feat(fx): matrix store + convert/getrates service (§112, ADR-125)"
```

---

### Task 4: `internal/fx` — OXR engine adapter (`remittance.RateProvider`)

**Files:**
- Create: `internal/fx/adapter.go`
- Test: `internal/fx/adapter_test.go`

**Interfaces:**
- Consumes: `remittance.RateProvider` (`FetchRates(ctx, apiConfig []byte, corridors []remittance.Corridor) ([]remittance.RateSnapshot, error)` — `internal/remittance/service.go:68`), `remittance.Corridor{SendCurrency, ReceiveCurrency string}`, `remittance.RateSnapshot{SendCurrency, ReceiveCurrency string, Rate, FeeFlat, FeePct float64, TransferTime, SourceURL string}`, `(*Service).LastFetchTime`, `.InsertMatrix`.
- Produces:
  - `fx.NewAdapter(svc *Service, httpClient *http.Client) *Adapter` — satisfies `remittance.RateProvider`
  - `providerConfig` struct (api_config JSON shape) — consumed only inside fx
  - Used in Task 5 wiring: `remittanceService.SetProviders(map[string]remittance.RateProvider{"mid-market": fx.NewAdapter(fxService, http.DefaultClient)})`

- [ ] **Step 1: Write the failing adapter test**

`internal/fx/adapter_test.go` — httptest server fixture, DB-free by stubbing the service? The service methods hit the DB — instead structure `Adapter` so fetch+map is pure:

```go
package fx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jedafe/japa/internal/remittance"
)

func TestParseOXRResponse(t *testing.T) {
	body := `{"disclaimer":"x","license":"x","timestamp":1,"base":"USD","rates":{"NGN":1600.5,"GBP":0.79,"EUR":0.91}}`
	rates, err := parseOXRResponse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if rates["NGN"] != 1600.5 || rates["GBP"] != 0.79 {
		t.Fatalf("unexpected rates: %v", rates)
	}
}

func TestCorridorUnionAndEmission(t *testing.T) {
	m := &Matrix{Base: "USD", Rates: map[string]float64{"USD": 1, "NGN": 1600, "GBP": 0.8, "GHS": 15}}

	configCorridors := []string{"GBP->NGN", "USD->NGN", "EUR->NGN"} // EUR absent → skipped
	requested := []remittance.Corridor{
		{SendCurrency: "USD", ReceiveCurrency: "GHS"},
		{SendCurrency: "USD", ReceiveCurrency: "NGN"}, // dup of config pair → dedup
	}

	snaps := emitSnapshots(m, configCorridors, requested, "https://openexchangerates.org")
	if len(snaps) != 3 {
		t.Fatalf("expected 3 snapshots (GBP->NGN, USD->NGN, USD->GHS), got %d: %+v", len(snaps), snaps)
	}
	for _, s := range snaps {
		if s.SourceURL == "" {
			t.Fatal("snapshot missing SourceURL attribution")
		}
	}
	// GBP->NGN cross-rate: 1600/0.8 = 2000
	for _, s := range snaps {
		if s.SendCurrency == "GBP" && s.ReceiveCurrency == "NGN" && s.Rate != 2000 {
			t.Fatalf("GBP->NGN rate: got %v want 2000", s.Rate)
		}
	}
}

func TestThrottle(t *testing.T) {
	// min_interval_minutes=60, last fetch 30 min ago → throttled.
	if !throttled(time.Now().Add(-30*time.Minute), 60) {
		t.Fatal("expected throttled within interval")
	}
	if throttled(time.Now().Add(-90*time.Minute), 60) {
		t.Fatal("expected not throttled past interval")
	}
	if throttled(time.Time{}, 60) {
		t.Fatal("zero last-fetch must never throttle")
	}
}

func TestFetchRates_EndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("app_id") != "testkey" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"base":  "USD",
			"rates": map[string]float64{"NGN": 1600, "GBP": 0.8},
		})
	}))
	defer srv.Close()

	a := &Adapter{svc: nil, http: srv.Client()} // svc nil → pure fetch path test via fetchMatrix
	cfg := providerConfig{AppID: "testkey", Endpoint: srv.URL, BaseCurrency: "USD"}

	rates, err := a.fetchMatrix(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if rates["USD"] != 1 || rates["NGN"] != 1600 {
		t.Fatalf("matrix missing entries: %v", rates)
	}
}
```

- [ ] **Step 2: Run — verify fail**

```bash
go test ./internal/fx/ -v
```

- [ ] **Step 3: Write `internal/fx/adapter.go`**

```go
package fx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/remittance"
)

// defaultOXREndpoint is the compiled-in endpoint for the oxr engine;
// overridable per-provider via api_config.endpoint (§112).
const defaultOXREndpoint = "https://openexchangerates.org/api/latest.json"

// attributionURL is rendered on benchmark rows and in the UI (OXR free tier
// terms require attribution).
const attributionURL = "https://openexchangerates.org"

// providerConfig is the decrypted api_config shape for fx engines (§112).
type providerConfig struct {
	Engine             string   `json:"engine"`
	AppID              string   `json:"app_id"`
	BaseCurrency       string   `json:"base_currency"`
	MinIntervalMinutes int      `json:"min_interval_minutes"`
	Endpoint           string   `json:"endpoint"`
	Corridors          []string `json:"corridors"`
}

// Adapter implements remittance.RateProvider for mid-market FX engines.
// One upstream call per remittance:fetch_rates tick writes the fx_rates
// matrix AND returns corridor snapshots for the remittance insert loop.
type Adapter struct {
	svc  *Service
	http *http.Client
}

// NewAdapter creates the adapter. httpClient may be nil → http.DefaultClient.
func NewAdapter(svc *Service, httpClient *http.Client) *Adapter {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Adapter{svc: svc, http: httpClient}
}

// FetchRates satisfies remittance.RateProvider (ADR-075 §2).
// apiConfig is the decrypted remittance_providers.api_config JSON.
// corridors are the platform's existing corridor pairs (may be empty).
func (a *Adapter) FetchRates(ctx context.Context, apiConfig []byte, corridors []remittance.Corridor) ([]remittance.RateSnapshot, error) {
	var cfg providerConfig
	if len(apiConfig) > 0 {
		if err := json.Unmarshal(apiConfig, &cfg); err != nil {
			return nil, fmt.Errorf("fx: parse api_config: %w", err)
		}
	}
	if cfg.Engine == "" {
		cfg.Engine = "oxr"
	}
	if cfg.BaseCurrency == "" {
		cfg.BaseCurrency = "USD"
	}
	if cfg.MinIntervalMinutes <= 0 {
		cfg.MinIntervalMinutes = 60
	}
	if cfg.AppID == "" {
		return nil, fmt.Errorf("fx: provider api_config missing app_id")
	}

	// Quota guard: skip when the last matrix is fresher than min_interval.
	if a.svc != nil {
		last, err := a.svc.LastFetchTime(ctx, cfg.Engine)
		if err != nil {
			slog.Warn("fx: last fetch probe failed, proceeding", "error", err)
		} else if throttled(last, cfg.MinIntervalMinutes) {
			slog.Info("fx: fetch throttled inside min_interval", "engine", cfg.Engine)
			return nil, nil
		}
	}

	rates, err := a.fetchMatrix(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if len(rates) == 0 {
		return nil, fmt.Errorf("fx: engine %q returned empty matrix", cfg.Engine)
	}

	if a.svc != nil {
		if err := a.svc.InsertMatrix(ctx, cfg.BaseCurrency, cfg.Engine, rates); err != nil {
			return nil, err
		}
	}

	m := &Matrix{Base: cfg.BaseCurrency, Rates: rates, FetchedAt: time.Now()}
	return emitSnapshots(m, cfg.Corridors, corridors, attributionURL), nil
}

// throttled reports whether lastFetch is inside minInterval minutes.
// A zero lastFetch (never fetched) never throttles.
func throttled(lastFetch time.Time, minIntervalMinutes int) bool {
	if lastFetch.IsZero() {
		return false
	}
	return time.Since(lastFetch) < time.Duration(minIntervalMinutes)*time.Minute
}

// fetchMatrix calls the engine endpoint and returns base→quote rates.
// The base→base = 1.0 cell is injected when the engine omits it so every
// matrix uniformly contains its own base.
func (a *Adapter) fetchMatrix(ctx context.Context, cfg providerConfig) (map[string]float64, error) {
	var rates map[string]float64
	var err error

	switch cfg.Engine {
	case "oxr":
		rates, err = a.fetchOXR(ctx, cfg)
	default:
		return nil, fmt.Errorf("fx: unknown engine %q", cfg.Engine)
	}
	if err != nil {
		return nil, err
	}
	if _, ok := rates[cfg.BaseCurrency]; !ok {
		rates[cfg.BaseCurrency] = 1
	}

	return rates, nil
}

func (a *Adapter) fetchOXR(ctx context.Context, cfg providerConfig) (map[string]float64, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = defaultOXREndpoint
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		endpoint+"?app_id="+cfg.AppID, nil)
	if err != nil {
		return nil, fmt.Errorf("fx: build oxr request: %w", err)
	}

	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fx: oxr request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fx: oxr status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("fx: read oxr body: %w", err)
	}

	return parseOXRResponse(body)
}

// parseOXRResponse extracts the rates map from a latest.json payload.
func parseOXRResponse(body []byte) (map[string]float64, error) {
	var parsed struct {
		Base  string             `json:"base"`
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("fx: parse oxr response: %w", err)
	}
	if parsed.Rates == nil {
		return nil, fmt.Errorf("fx: oxr response has no rates")
	}
	return parsed.Rates, nil
}

// emitSnapshots converts a matrix into per-corridor RateSnapshots for the
// union of config corridors ("FROM->TO" strings) and requested corridors,
// deduplicated and limited to pairs the matrix covers.
func emitSnapshots(m *Matrix, configCorridors []string, requested []remittance.Corridor, sourceURL string) []remittance.RateSnapshot {
	seen := map[string]bool{}
	var pairs []remittance.Corridor

	for _, c := range configCorridors {
		parts := strings.SplitN(c, "->", 2)
		if len(parts) != 2 {
			continue
		}
		pairs = append(pairs, remittance.Corridor{
			SendCurrency:    strings.TrimSpace(parts[0]),
			ReceiveCurrency: strings.TrimSpace(parts[1]),
		})
	}
	pairs = append(pairs, requested...)

	var snaps []remittance.RateSnapshot
	for _, p := range pairs {
		key := p.SendCurrency + "->" + p.ReceiveCurrency
		if seen[key] {
			continue
		}
		seen[key] = true

		fromRate, ok1 := m.Rates[p.SendCurrency]
		toRate, ok2 := m.Rates[p.ReceiveCurrency]
		if !ok1 || !ok2 || fromRate == 0 {
			continue
		}

		snaps = append(snaps, remittance.RateSnapshot{
			SendCurrency:    p.SendCurrency,
			ReceiveCurrency: p.ReceiveCurrency,
			Rate:            toRate / fromRate,
			SourceURL:       sourceURL,
		})
	}

	return snaps
}
```

- [ ] **Step 4: Run tests — verify pass**

```bash
go test ./internal/fx/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/fx/adapter.go internal/fx/adapter_test.go
git commit -m "feat(fx): OXR engine adapter — throttled matrix fetch + corridor snapshots (§112, ADR-125)"
```

---

### Task 5: `internal/fx` — handlers, routes, flag, server wiring

**Files:**
- Create: `internal/fx/handlers.go`
- Modify: `internal/features/registry.go` (add `fx.enabled` — place next to `remittance.enabled` ~line 91)
- Modify: `internal/server/server.go` (~lines 1556–1568 wiring, ~line 7094 route block)

**Interfaces:**
- Consumes: `fx.NewService`, `fx.NewAdapter`, `errpkg.WriteError/WriteJSON`, `features.Gate`, `config.Settings`.
- Produces: `fx.NewHandlers(service *Service) *Handlers` with `HandleConvert`, `HandleGetRates`, `HandleCurrencies`; routes `GET /api/v1/fx/{convert,rates,currencies}`.

- [ ] **Step 1: Add the feature flag to the registry**

In `internal/features/registry.go`, after `{Key: "remittance.enabled", Label: "Remittance Rate Tracker", Default: true},`:

```go
	{Key: "fx.enabled", Label: "FX Currency Converter", Default: true},
```

- [ ] **Step 2: Write `internal/fx/handlers.go`**

Mirror `internal/remittance/handlers.go` style:

```go
// Package fx — handlers.go provides the HTTP handlers for the FX Currency
// Converter routes (Section 110, ADR-125).
//
// Public routes (no auth):
//
//	GET /api/v1/fx/convert     — ?amount=&from=&to=
//	GET /api/v1/fx/rates       — ?base=USD
//	GET /api/v1/fx/currencies  — picker list
//
// Ref: japaconnect-engineering/sections/112-fx-converter.md
//
//	japaconnect-engineering/adr/ADR-125-fx-converter.md
package fx

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the FX converter HTTP endpoints.
type Handlers struct {
	service *Service
}

// NewHandlers creates fx handlers bound to the provided service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// writeServiceErr maps service errors to the standard error envelope (Rule 5).
func writeServiceErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnsupportedCurrency):
		errpkg.WriteError(w, http.StatusBadRequest, "unsupported_currency", "Currency not supported")
	case errors.Is(err, ErrEmptyMatrix):
		errpkg.WriteError(w, http.StatusServiceUnavailable, "fx_unavailable", "FX rates are not available yet")
	default:
		slog.Error("fx: handler failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "fx_failed", "FX request failed")
	}
}

// HandleConvert handles GET /api/v1/fx/convert — ?amount=&from=&to=.
func (h *Handlers) HandleConvert(w http.ResponseWriter, r *http.Request) {
	amountStr := r.URL.Query().Get("amount")
	from := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("from")))
	to := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("to")))

	if amountStr == "" || from == "" || to == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "amount, from, and to query params are required")
		return
	}

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_amount", "amount must be a positive number")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "fx_unavailable", "FX service not configured")
		return
	}

	result, err := h.service.Convert(r.Context(), amount, from, to)
	if err != nil {
		writeServiceErr(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// HandleGetRates handles GET /api/v1/fx/rates — ?base=USD (defaults to the
// configured base when omitted).
func (h *Handlers) HandleGetRates(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "fx_unavailable", "FX service not configured")
		return
	}

	base := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("base")))
	if base == "" {
		base = h.service.DefaultBaseCurrency(r.Context())
	}

	m, stale, err := h.service.GetRates(r.Context(), base)
	if err != nil {
		writeServiceErr(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"base":       m.Base,
		"rates":      m.Rates,
		"fetched_at": m.FetchedAt.UTC().Format("2006-01-02T15:04:05Z"),
		"stale":      stale,
	}})
}

// HandleCurrencies handles GET /api/v1/fx/currencies — picker list.
func (h *Handlers) HandleCurrencies(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "fx_unavailable", "FX service not configured")
		return
	}

	rows, err := h.service.ListCurrencies(r.Context())
	if err != nil {
		writeServiceErr(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}
```

- [ ] **Step 3: Wire services + adapter + routes in `internal/server/server.go`**

a) After the remittance wiring block (~line 1556–1568), add — need `import "github.com/jedafe/japa/internal/fx"` in the import block too:

```go
	// §112 / ADR-125: FX converter — matrix service + the rate-provider
	// adapter that feeds the seeded 'mid-market' benchmark provider.
	fxService := fx.NewService(fx.NewStore(database), settingsService)
	fxService.SetFeatureChecker(featureChecker)
	fxService.SetCache(vkClient.Redis())
	fxHandlers := fx.NewHandlers(fxService)
	remittanceService.SetProviders(map[string]remittance.RateProvider{
		"mid-market": fx.NewAdapter(fxService, nil),
	})
```

b) Route block — inside the same `r.With(features.Gate(...)).Group` region used by remittance (line ~7094), add a sibling group immediately after the remittance group closes (~line 7118):

```go
			r.With(features.Gate(featureChecker, "fx.enabled")).Group(func(r chi.Router) {
				r.Get("/fx/convert", fxHandlers.HandleConvert)
				r.Get("/fx/rates", fxHandlers.HandleGetRates)
				r.Get("/fx/currencies", fxHandlers.HandleCurrencies)
			})
```

c) `RegisterV2Processors` call site gains an `fxService` argument — update the call (~wherever `workerv2.RegisterV2Processors(...)` is invoked) to pass it. Task 7 adds the parameter.

- [ ] **Step 4: Build + smoke-check route registration**

```bash
go build ./...
grep -n "fx/convert" internal/server/server.go
```

- [ ] **Step 5: Commit**

```bash
git add internal/fx/handlers.go internal/features/registry.go internal/server/server.go
git commit -m "feat(fx): public convert/rates/currencies routes + mid-market provider wiring (§112, ADR-125)"
```

---

### Task 6: `internal/remittance` — benchmark discipline

**Files:**
- Modify: `internal/remittance/service.go` (DTOs, `GetRates`, `GetBestRate`, `Calculate`, `computeProviderCalculation`)
- Modify: `internal/remittance/handlers.go` (`is_benchmark` in provider create/update bodies)
- Test: `internal/remittance/service_test.go` (new file or extend existing `handlers_test.go` style)

**Interfaces:**
- Consumes: `gen.GetLatestRatesForCorridorRow.ProviderIsBenchmark`, `gen.RemittanceProvider.IsBenchmark`, `gen.Create/UpdateRemittanceProviderParams.IsBenchmark` (Task 2 sqlc output).
- Produces:
  - `RateInfo.IsBenchmark bool \`json:"is_benchmark"\``
  - `ProviderInfo.IsBenchmark bool \`json:"is_benchmark"\``
  - `CalculateResult.Benchmark *BenchmarkResult \`json:"benchmark,omitempty"\``
  - `type BenchmarkResult struct { Rate float64 \`json:"rate"\`; ReceiveAmount float64 \`json:"receive_amount"\` }`

- [ ] **Step 1: Write the failing test**

`internal/remittance/service_test.go` — test the pure ranking path by extracting selection logic? `Calculate` needs a store — instead test `computeProviderCalculation` exclusion via a new unexported helper `splitBenchmark(rates []RateInfo) (providers []RateInfo, benchmark *RateInfo)`:

```go
package remittance

import "testing"

func TestSplitBenchmark(t *testing.T) {
	rates := []RateInfo{
		{ProviderID: "a", ProviderName: "Provider A", Rate: 1500},
		{ProviderID: "mm", ProviderName: "Mid-market", Rate: 1540, IsBenchmark: true},
		{ProviderID: "b", ProviderName: "Provider B", Rate: 1480},
	}
	providers, bench := splitBenchmark(rates)
	if len(providers) != 2 {
		t.Fatalf("expected 2 provider rows, got %d", len(providers))
	}
	if bench == nil || bench.ProviderID != "mm" {
		t.Fatalf("expected benchmark row 'mm', got %+v", bench)
	}
	for _, p := range providers {
		if p.IsBenchmark {
			t.Fatal("benchmark row leaked into provider list")
		}
	}
}

func TestSplitBenchmark_None(t *testing.T) {
	providers, bench := splitBenchmark([]RateInfo{{ProviderID: "a", Rate: 1}})
	if bench != nil || len(providers) != 1 {
		t.Fatal("no-benchmark input must pass through unchanged")
	}
}
```

- [ ] **Step 2: Run — verify fail**

```bash
go test ./internal/remittance/ -run TestSplitBenchmark -v
```

- [ ] **Step 3: Implement**

In `internal/remittance/service.go`:

a) Add fields to DTOs (~lines 131–194):

```go
// in RateInfo:
	IsBenchmark     bool              `json:"is_benchmark"`

// in ProviderInfo:
	IsBenchmark  bool   `json:"is_benchmark"`

// new type near CalculateResult:
type BenchmarkResult struct {
	Rate          float64 `json:"rate"`
	ReceiveAmount float64 `json:"receive_amount"`
}

// in CalculateResult:
	Benchmark         *BenchmarkResult      `json:"benchmark,omitempty"`
```

b) In `GetRates` row-mapping loop (~line 420): set `IsBenchmark: r.ProviderIsBenchmark`.

c) In `ProviderInfo` construction (find the `GetProviders`/`ListProvidersPaginated` mapping — search `ProviderName:` in service.go): set `IsBenchmark: p.IsBenchmark` from the gen row.

d) Add `splitBenchmark` helper and use it in `GetBestRate` + `Calculate`:

```go
// splitBenchmark partitions corridor rates into sellable provider rows and
// the (at most one) benchmark row (§112/ADR-125). Benchmark rates are a
// mid-market reference — never ranked against provider quotes.
func splitBenchmark(rates []RateInfo) (providers []RateInfo, benchmark *RateInfo) {
	for i := range rates {
		if rates[i].IsBenchmark {
			if benchmark == nil || rates[i].Rate > benchmark.Rate {
				cp := rates[i]
				benchmark = &cp
			}
			continue
		}
		providers = append(providers, rates[i])
	}
	return providers, benchmark
}
```

In `GetBestRate` (~line 473): replace `rates, err := s.GetRates(...)`'s use — after fetching, do `providers, _ := splitBenchmark(rates)` and iterate `providers` instead of `rates`. Early-return `nil` when `len(providers) == 0`.

In `Calculate` (~line 519): same split; build `providers` list from the non-benchmark rows; after computing the ranked list, if `bench != nil`:

```go
	bc := computeProviderCalculation(*bench, sendAmount)
	result.Benchmark = &BenchmarkResult{Rate: bc.Rate, ReceiveAmount: bc.ReceiveAmount}
```

e) In `handlers.go`: add `IsBenchmark *bool \`json:"is_benchmark"\`` to the update-provider body struct (~line 566) and `IsBenchmark bool \`json:"is_benchmark"\`` to the create body (~line 493); pass `body.IsBenchmark` into `gen.UpdateRemittanceProviderParams.IsBenchmark` / `gen.CreateRemittanceProviderParams.IsBenchmark`.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/remittance/ -v
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/remittance/
git commit -m "feat(remittance): benchmark providers excluded from ranking, exposed as reference (§112, ADR-125)"
```

---

### Task 7: Worker — `fx:prune_rates` end to end

**Files:**
- Modify: `internal/worker/v2_processors.go` (const, `FxProcessor` interface, `fxProcessor` var, `RegisterFxProcessor`, `HandleFxPruneRates`)
- Modify: `internal/worker/tasks.go` (handler table entry ~line 288)
- Modify: `internal/worker/enqueuer.go` (`taskTier` low list + `taskTypeCatalog` entry)
- Modify: `internal/worker/server.go` (schedule entry ~line 294)
- Modify: `internal/workerv2/adapters.go` (`FxAdapter` + `RegisterV2Processors` signature)
- Modify: `internal/server/server.go` (`RegisterV2Processors` call site — add `fxService` arg)

**Interfaces:**
- Consumes: `(*fx.Service).PruneRates(ctx) (int, error)` (Task 3).
- Produces: `worker.TypeFxPruneRates = "fx:prune_rates"`; `worker.FxProcessor{PruneRates(ctx)(int,error)}`; `worker.RegisterFxProcessor`; `workerv2.NewFxAdapter(*fx.Service)`.

- [ ] **Step 1: Add task type + processor plumbing in `internal/worker/v2_processors.go`**

In the const block near `TypeRemittancePruneOldRates` (~line 391):

```go
	// TypeFxPruneRates — §112 FX Currency Converter (ADR-125).
	// fx:prune_rates runs weekly on the low queue, deleting fx_rates matrix
	// rows past fx.history_retention_days (default 90) with daily downsampling.
	TypeFxPruneRates = "fx:prune_rates"
```

In the processor interfaces block (~line 66, after `RemittanceProcessor`):

```go
// FxProcessor handles FX converter background tasks (§112, ADR-125).
type FxProcessor interface {
	PruneRates(ctx context.Context) (int, error)
}
```

Add the slot var next to `remittanceProcessor` (~line 279):

```go
	fxProcessor             FxProcessor
```

Add the register func next to `RegisterRemittanceProcessor` (~line 318):

```go
// RegisterFxProcessor is an exported function.
func RegisterFxProcessor(p FxProcessor) { fxProcessor = p }
```

Add the handler after `HandleRemittancePruneOldRates` (~line 710):

```go
// HandleFxPruneRates removes fx_rates rows beyond retention (§112).
func HandleFxPruneRates(ctx context.Context, _ *asynq.Task) error {
	return countTask(ctx, "fx:prune_rates", "pruned fx matrices", fxProcessor != nil,
		func(ctx context.Context) (int, error) { return fxProcessor.PruneRates(ctx) })
}
```

- [ ] **Step 2: Register handler, queue tier, catalog, schedule**

a) `internal/worker/tasks.go` — after `{TypeRemittancePruneOldRates, HandleRemittancePruneOldRates},`:

```go
	{TypeFxPruneRates, HandleFxPruneRates},
```

b) `internal/worker/enqueuer.go` — in the `taskTier` low-queue list (next to `TypeRemittancePruneOldRates` ~line 294):

```go
		TypeFxPruneRates,
```

and in `taskTypeCatalog` (next to the remittance prune row ~line 543):

```go
	{TypeFxPruneRates, "low", "Weekly — delete fx_rates matrix rows past fx.history_retention_days (§112)"},
```

c) `internal/worker/server.go` — after the remittance prune schedule (~line 294):

```go
	// Weekly FX matrix pruning — Sunday 03:30 UTC (§112, ADR-125).
	// Deletes fx_rates rows older than fx.history_retention_days (default 90).
	// Low queue.
	schedule("30 3 * * 0", TypeFxPruneRates, 7*24*time.Hour)
```

- [ ] **Step 3: `internal/workerv2/adapters.go` — FxAdapter + registration**

```go
// ---------------------------------------------------------------------------
// FxAdapter — wraps fx.Service (§112, ADR-125).
// ---------------------------------------------------------------------------

// FxAdapter adapts fx.Service to the worker.FxProcessor interface.
type FxAdapter struct {
	svc *fx.Service
}

// NewFxAdapter creates an FxAdapter.
func NewFxAdapter(svc *fx.Service) *FxAdapter {
	return &FxAdapter{svc: svc}
}

// PruneRates deletes fx_rates rows past fx.history_retention_days (§112).
func (a *FxAdapter) PruneRates(ctx context.Context) (int, error) {
	return a.svc.PruneRates(ctx)
}
```

Import `"github.com/jedafe/japa/internal/fx"`. In `RegisterV2Processors`: add param `fxSvc *fx.Service` and line:

```go
	worker.RegisterFxProcessor(NewFxAdapter(fxSvc))
```

Update the `slog.Info` to include `"fx", true`.

- [ ] **Step 4: Update the call site in `internal/server/server.go`**

Find `workerv2.RegisterV2Processors(` — add `fxService` at the matching position (keep argument order consistent with the function signature — put `fxSvc` right after `remSvc` if that's where it was added).

- [ ] **Step 5: Build + commit**

```bash
go build ./...
git add internal/worker/ internal/workerv2/ internal/server/server.go
git commit -m "feat(worker): fx:prune_rates weekly retention sweep, low queue (§112, ADR-125)"
```

---

### Task 8: `apps/admin` — benchmark flag on provider UI

**Files:**
- Modify: `apps/admin/src/routes/(app)/remittance/+page.svelte` (legacy hand-rolled page — add `is_benchmark` to the provider type, forms, and a table badge)
- Modify: `apps/admin/src/lib/locales/en.json` + `fr.json` if the page uses `t()` keys for the new label (follow existing `remittance.*` key pattern)

**Interfaces:**
- Consumes: `is_benchmark` on provider list/create/update payloads (Task 6 backend).
- Produces: admin can flag a provider as benchmark; benchmark rows show a badge.

- [ ] **Step 1: Add `is_benchmark` to the page's provider interface + forms**

In `apps/admin/src/routes/(app)/remittance/+page.svelte`:

- Provider interface (~line 18): add `is_benchmark: boolean;`
- `newProvider` initializer (~line 41): add `is_benchmark: false`
- Create + edit payloads: pass `is_benchmark` through (the update body ~line 96 and create body — find where `is_active` is sent and add alongside)
- Provider table columns (~line 157 area): add a column `{ key: 'is_benchmark', label: t('remittance.is_benchmark', 'Benchmark') }` rendering a badge — mirror how `is_active` is rendered in this file
- Create form (~line 268): add checkbox `<input type="checkbox" id="remittance-benchmark" bind:checked={newProvider.is_benchmark} />` with label `t('remittance.is_benchmark', 'Benchmark (mid-market reference)')`
- Edit form (~line 299): same checkbox bound to `editingProvider.is_benchmark`

- [ ] **Step 2: i18n keys**

Add to `apps/admin/src/lib/locales/en.json` (near `"app.remittance.*"` keys): `"app.remittance.is_benchmark": "Benchmark (mid-market reference)"`. Add the `fr.json` equivalent: `"app.remittance.is_benchmark": "Référence (taux interbancaire)"`. Match the exact key naming the page's `t()` calls use (verify — the file greps showed `t('remittance.xxx')` vs locale keys `app.remittance.*`; check how `t()` resolves and use the same prefix).

- [ ] **Step 3: Check + commit**

```bash
cd apps/admin && bun run check
git add apps/admin/
git commit -m "feat(admin): is_benchmark flag on remittance provider forms/list (§112)"
```

---

### Task 9: `apps/tools` — currency converter page + surface wiring

**Files:**
- Modify: `apps/tools/src/lib/types/index.ts` (`ToolSurface` union + new `FxCurrency`/`FxConvertResult` types)
- Modify: `apps/tools/src/lib/tools/registry.ts` (registry entry)
- Modify: `apps/tools/src/hooks.server.ts` (`TOOL_SETTINGS_KEYS` + `DEFAULT_SUBDOMAINS`)
- Create: `apps/tools/src/routes/currency-converter/+page.server.ts`
- Create: `apps/tools/src/routes/currency-converter/+page.svelte`
- Modify: `apps/tools/src/lib/i18n/en.json`, `fr.json`

**Interfaces:**
- Consumes: `GET /fx/currencies` → `{code,name,symbol}[]` (apiList `.items`); `GET /fx/convert?amount&from&to` → `FxConvertResult`; `getTool`, `isToolEnabled`, `apiContext`, `apiList` (same helpers as remittance page).
- Produces: route `/currency-converter`, tool slug `'currency-converter'`, settingsKey `'fx'`.

- [ ] **Step 1: Types**

In `apps/tools/src/lib/types/index.ts`:

```ts
// in the ToolSurface union, after 'sop-generator':
	| 'currency-converter';
```

(Remove the `;` from `'sop-generator'` line accordingly.)

Append new interfaces:

```ts
/** §112 FX Currency Converter — picker currency row. */
export interface FxCurrency {
	code: string;
	name: string;
	symbol: string;
}

/** §112 FX Currency Converter — /fx/convert response. */
export interface FxConvertResult {
	amount: number;
	from: string;
	to: string;
	rate: number;
	converted: number;
	fetched_at: string;
	stale: boolean;
}
```

Also add `is_benchmark?: boolean` to `RemittanceRate` (~line 386).

- [ ] **Step 2: Registry entry**

In `apps/tools/src/lib/tools/registry.ts`, append to `TOOLS` (after `sop-generator`):

```ts
	{
		slug: 'currency-converter',
		route: '/currency-converter',
		icon: 'coins',
		name: 'Currency Converter',
		nameKey: 'tool.fx_converter.name',
		tagline: 'Mid-market rates for 160+ currencies',
		taglineKey: 'tool.fx_converter.tagline',
		description:
			'Convert between 160+ currencies at live mid-market rates — including NGN, GHS, KES — powered by Open Exchange Rates.',
		descriptionKey: 'tool.fx_converter.description',
		accent: '#14b8a6',
		accentHover: '#0d9488',
		settingsKey: 'fx',
		enabledSetting: 'fx.enabled',
		keywords: ['currency converter', 'exchange rate', 'fx', 'NGN', 'mid-market']
	}
```

- [ ] **Step 3: Host detection**

In `apps/tools/src/hooks.server.ts`:

- `TOOL_SETTINGS_KEYS` (~line 26): add `{ key: 'fx', surface: 'currency-converter' },`
- `DEFAULT_SUBDOMAINS` (~line 52): add `fx: 'currency-converter',`

- [ ] **Step 4: `+page.server.ts`**

Mirror `apps/tools/src/routes/remittance/+page.server.ts` gating pattern:

```ts
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { FxCurrency } from '$lib/types';

/**
 * FX Currency Converter — §112, ADR-125.
 * SSR loads the picker currency list; conversion happens client-side.
 *
 * API: GET /fx/currencies — active currencies with matrix coverage.
 *      GET /fx/convert?amount=&from=&to= — mid-market conversion.
 *
 * Ref: §112 (FX Currency Converter).
 */
export const load: PageServerLoad = async (event) => {
	const tool = getTool('currency-converter')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return { enabled: false, tool, currencies: [], user: event.locals.user };
	}

	const ctx = apiContext(event);

	let currencies: FxCurrency[] = [];
	try {
		currencies = (await apiList<FxCurrency>(ctx, '/fx/currencies')).items;
	} catch {
		currencies = [];
	}

	return { enabled: true, tool, currencies, user: event.locals.user };
};
```

- [ ] **Step 5: `+page.svelte`**

Complete page, following the remittance page's conventions (untrack + `$state`, `api` client, `t()` i18n, `Icon`, `ToolDisabled`, `buildSeoMeta`, `AutocompleteSelect`, debounced calls, `formatMoney`/`currencyLabel` from `$lib/utils/currency`):

```svelte
<script lang="ts">
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { formatMoney, currencyLabel } from '$lib/utils/currency';
	import type { FxCurrency, FxConvertResult } from '$lib/types';

	/**
	 * FX Currency Converter — §112, ADR-125.
	 * Any-to-any mid-market conversion: amount + from/to pickers → result card.
	 * Data: /fx/currencies (SSR) + /fx/convert (client, debounced).
	 */
	let { data } = $props();

	let currencies = $state<FxCurrency[]>(untrack(() => data.currencies));
	let amount = $state(1000);
	let from = $state('USD');
	let to = $state('NGN');
	let result = $state<FxConvertResult | null>(null);
	let loading = $state(false);
	let convertError = $state('');
	let timer: ReturnType<typeof setTimeout> | null = null;

	const seo = $derived(
		buildSeoMeta({
			title: t('fx.seo_title', 'Currency Converter — JapaConnect Tools'),
			description: t(
				'fx.seo_description',
				'Convert between 160+ currencies at live mid-market rates.'
			),
			path: '/currency-converter',
			origin: page.url.origin
		})
	);

	const inverseRate = $derived(result && result.rate > 0 ? 1 / result.rate : null);

	async function convert() {
		if (amount <= 0 || !from || !to) return;
		loading = true;
		convertError = '';
		const res = await api.get<FxConvertResult>(
			`/fx/convert?amount=${amount}&from=${from}&to=${to}`
		);
		loading = false;
		if (res.error) {
			convertError = res.error.message || t('fx.error', 'Conversion failed');
			result = null;
			return;
		}
		result = res.data;
	}

	/** Debounced convert — waits 400ms after the last keystroke (§4.1). */
	function debouncedConvert() {
		if (timer) clearTimeout(timer);
		timer = setTimeout(() => convert(), 400);
	}

	function swap() {
		[from, to] = [to, from];
		convert();
	}

	const currencyOptions = $derived(
		currencies.map((c) => ({ value: c.code, label: `${c.code} — ${c.name}`, sublabel: c.symbol }))
	);

	// Initial conversion once currencies arrive.
	$effect(() => {
		if (currencies.length > 0 && !result && !loading) {
			untrack(() => convert());
		}
	});
</script>

<svelte:head>
	{#if seo}
		<title>{seo.title}</title>
		<meta name="description" content={seo.description} />
	{/if}
</svelte:head>

{#if !data.enabled}
	<ToolDisabled tool={data.tool} />
{:else}
	<section class="fx-page">
		<header class="fx-head">
			<h1>{t('fx.title', 'Currency Converter')}</h1>
			<p class="fx-sub">{t('fx.subtitle', 'Live mid-market rates across 160+ currencies.')}</p>
		</header>

		<div class="fx-card">
			<div class="fx-row">
				<label class="fx-field">
					<span>{t('fx.amount', 'Amount')}</span>
					<input
						type="number"
						min="0"
						bind:value={amount}
						oninput={debouncedConvert}
						class="fx-input"
					/>
				</label>
			</div>

			<div class="fx-pair">
				<label class="fx-field">
					<span>{t('fx.from', 'From')}</span>
					<AutocompleteSelect
						options={currencyOptions}
						bind:value={from}
						onValueChange={() => convert()}
					/>
				</label>
				<button class="fx-swap" onclick={swap} aria-label={t('fx.swap', 'Swap currencies')}>
					<Icon name="arrow-left-right" size={18} />
				</button>
				<label class="fx-field">
					<span>{t('fx.to', 'To')}</span>
					<AutocompleteSelect
						options={currencyOptions}
						bind:value={to}
						onValueChange={() => convert()}
					/>
				</label>
			</div>

			<div class="fx-result" aria-live="polite">
				{#if loading}
					<p class="fx-loading">{t('fx.loading', 'Converting…')}</p>
				{:else if convertError}
					<p class="fx-error">{convertError}</p>
				{:else if result}
					<p class="fx-amount">{formatMoney(result.converted, result.to)}</p>
					<p class="fx-rate">
						1 {result.from} = {result.rate.toFixed(4)} {result.to}
						{#if inverseRate}
							· 1 {result.to} = {inverseRate.toFixed(4)} {result.from}
						{/if}
					</p>
					<p class="fx-meta">
						{t('fx.updated', 'Updated')} {new Date(result.fetched_at).toLocaleString()}
						{#if result.stale}
							<span class="fx-stale">{t('fx.stale', 'rates may be stale')}</span>
						{/if}
					</p>
				{/if}
			</div>
		</div>

		<footer class="fx-attribution">
			<a href="https://openexchangerates.org" rel="noopener noreferrer" target="_blank">
				{t('fx.attribution', 'Rates by Open Exchange Rates')}
			</a>
		</footer>
	</section>
{/if}

<style>
	.fx-page { max-width: 640px; margin: 0 auto; padding: 2rem 1rem; }
	.fx-head { margin-bottom: 1.5rem; }
	.fx-sub { color: var(--color-muted); }
	.fx-card {
		border: 1px solid var(--color-border);
		border-radius: 12px;
		padding: 1.25rem;
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}
	.fx-pair { display: grid; grid-template-columns: 1fr auto 1fr; gap: 0.75rem; align-items: end; }
	.fx-field { display: flex; flex-direction: column; gap: 0.35rem; font-size: 0.875rem; }
	.fx-input {
		padding: 0.6rem 0.75rem;
		border: 1px solid var(--color-border);
		border-radius: 8px;
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 1rem;
	}
	.fx-swap {
		padding: 0.6rem;
		border: 1px solid var(--color-border);
		border-radius: 8px;
		background: transparent;
		color: var(--color-muted);
		cursor: pointer;
	}
	.fx-result { min-height: 5.5rem; }
	.fx-amount { font-size: 1.75rem; font-weight: 700; }
	.fx-rate { color: var(--color-muted); }
	.fx-meta { font-size: 0.8rem; color: var(--color-muted); }
	.fx-stale { color: var(--color-warning, #d97706); margin-left: 0.5rem; }
	.fx-error { color: var(--color-danger, #dc2626); }
	.fx-attribution { margin-top: 1.5rem; font-size: 0.8rem; text-align: center; }
	.fx-attribution a { color: var(--color-muted); }
</style>
```

`AutocompleteSelect` (from `@japaconnect/ui`) takes `options: {value,label,sublabel}[]`, `bind:value`, and `onValueChange(v)` — the code above already matches that contract. `formatMoney(amount, code)` matches `$lib/utils/currency`'s signature.

- [ ] **Step 6: i18n keys**

`apps/tools/src/lib/i18n/en.json` — add (group near existing `tool.*` / `remittance.*` keys):

```json
	"tool.fx_converter.name": "Currency Converter",
	"tool.fx_converter.tagline": "Mid-market rates for 160+ currencies",
	"tool.fx_converter.description": "Convert between 160+ currencies at live mid-market rates — including NGN, GHS, KES — powered by Open Exchange Rates.",
	"fx.title": "Currency Converter",
	"fx.subtitle": "Live mid-market rates across 160+ currencies.",
	"fx.amount": "Amount",
	"fx.from": "From",
	"fx.to": "To",
	"fx.swap": "Swap currencies",
	"fx.loading": "Converting…",
	"fx.error": "Conversion failed",
	"fx.updated": "Updated",
	"fx.stale": "rates may be stale",
	"fx.attribution": "Rates by Open Exchange Rates",
	"fx.seo_title": "Currency Converter — JapaConnect Tools",
	"fx.seo_description": "Convert between 160+ currencies at live mid-market rates."
```

`fr.json` — French equivalents (name "Convertisseur de devises", etc.).

- [ ] **Step 7: Check + commit**

```bash
cd apps/tools && bun run check
git add apps/tools/
git commit -m "feat(tools): currency converter tool page + surface wiring (§112)"
```

---

### Task 10: `apps/main` — converter page + benchmark badge on remittance

**Files:**
- Create: `apps/main/src/routes/(app)/tools/currency-converter/+page.server.ts`
- Create: `apps/main/src/routes/(app)/tools/currency-converter/+page.svelte`
- Modify: `apps/main/src/lib/tools/registry.ts` (tools-hub entry — reviewed: main app has its OWN registry driving the `/tools` card grid; without an entry the tool is invisible and ungated there)
- Modify: `apps/main/src/lib/i18n/en.json` + `fr.json` (`tools.currency_converter` / `tools.currency_converter_desc` label keys used by the registry entry)
- Modify: `apps/main/src/lib/types.ts` (or wherever `RemittanceRate` lives — add `is_benchmark?: boolean` + `FxCurrency`/`FxConvertResult`; **fix `RemittanceCalculation.providers` → `results`** — reviewed: backend `CalculateResult` serializes `json:"results"`, so `calc.providers` is always empty and the whole calculator block silently falls back to corridor rates — pre-existing bug, fix while adding `benchmark`)
- Modify: `apps/main/src/routes/(app)/tools/remittance/+page.svelte` AND `+page.server.ts` (update `calc.providers` reads to `calc.results`; benchmark badge + exclude from best-rate display + benchmark reference line)
- Modify: `apps/tools/src/routes/remittance/+page.svelte` (same badge treatment — the tools page computes `bestRate` by `reduce` at line ~74; filter `!r.is_benchmark` first and render benchmark row with badge)

**Interfaces:**
- Consumes: `RateInfo.is_benchmark` + `CalculateResult.benchmark` (Task 6); `/fx/currencies`, `/fx/convert` (Task 5); main-app helpers `apiContext`, `apiGet`, `apiList`, `event.parent().features`.

- [ ] **Step 1: Main app types + tools-hub registry + i18n**

In the main app's types file (`apps/main/src/lib/types/index.ts`, `RemittanceCalculation` ~line 2734):
- `is_benchmark?: boolean` on `RemittanceRate` and on `RemittanceProviderCalc` rows.
- Add `FxCurrency`/`FxConvertResult` (same shapes as Task 9).
- Fix `RemittanceCalculation`: rename `providers: RemittanceProviderCalc[]` → `results: RemittanceProviderCalc[]` (matches backend `json:"results"`) and add `benchmark?: RemittanceProviderCalc | null`. Then update `+page.server.ts`/`+page.svelte` reads of `calc.providers` → `calc.results` (lines ~116, ~249-250, ~440 reference `calc.providers`/`calculation.providers` — all currently dead code).

In `apps/main/src/lib/tools/registry.ts`, add after the remittance entry (~line 161, finance section):

```ts
{
	href: '/tools/currency-converter',
	labelKey: 'tools.currency_converter',
	descKey: 'tools.currency_converter_desc',
	enabledSetting: 'fx.enabled',
	sections: ['finance', 'settlement']
},
```

In `apps/main/src/lib/i18n/en.json` (near `"tools.remittance"` ~line 4100): `"tools.currency_converter": "Currency Converter"`, `"tools.currency_converter_desc": "Convert between currencies at live mid-market rates."` — and `fr.json` equivalents near `"tools.remittance"` (~line 4050): `"Convertisseur de devises"`, `"Convertissez entre devises aux taux moyens du marché en direct."`.

`apps/main/src/routes/(app)/tools/currency-converter/+page.server.ts`:

```ts
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { FxCurrency } from '$lib/types';

/**
 * FX Currency Converter at /tools/currency-converter (§112, ADR-125).
 * SSR-fetches the picker currency list; conversion is client-side.
 *
 * Ref: japaconnect-engineering/sections/112-fx-converter.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['fx.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Currency Converter is not available' });
	}

	const ctx = apiContext(event);
	let currencies: FxCurrency[] = [];
	try {
		currencies = (await apiList<FxCurrency>(ctx, '/fx/currencies')).items;
	} catch {
		currencies = [];
	}

	return { currencies };
};
```

- [ ] **Step 2: `+page.svelte`**

Port the Task 9 page into the main app's component conventions — look at `(app)/tools/remittance/+page.svelte` for which UI primitives the main app uses (it may not share `AutocompleteSelect`/`ToolDisabled` with the tools app). Reuse the main app's existing select/component primitives; keep the same state logic (amount/from/to/debounced convert/swap/result card/attribution). Keep styling consistent with the main app's tool pages (read a simple sibling page like `(app)/tools/tax-calculator` if present for the canonical card structure — check `apps/main/src/routes/(app)/tools/` listing first and follow that page's layout pattern rather than inventing one).

- [ ] **Step 3: Benchmark badge + reference line on remittance pages**

`apps/main/src/routes/(app)/tools/remittance/+page.svelte` (and `+page.server.ts` where `calc.providers` is read):
- Rename `calc.providers`/`calculation.providers` reads to `calc.results`/`calculation.results` (part of the Step 1 fix — the calculator block has been dead code since the field never existed).
- Where the provider rate table renders rows, add a "Mid-market" badge when `rate.is_benchmark` (find the row render — search `provider_name` in the file).
- Where best rate is highlighted (`bestRate` reduce ~line 249), skip `is_benchmark` rows — filter `!p.is_benchmark` before the reduce.
- Where `RemittanceCalculation` result renders, render `calc.benchmark` as a reference line above the ranked results: `Mid-market would give X` using `formatMoney` (find the calculation render block — search `receive_amount`).

`apps/tools/src/routes/remittance/+page.svelte`:
- `bestRate` derived (~line 74): filter out benchmark rows first — `rates.filter(r => !r.is_benchmark).reduce(...)`.
- In the rates table render, add the badge for `r.is_benchmark` rows (search `provider_name` render).
- Where `calcResult` renders, add the same reference line from `calcResult.benchmark`.

Both apps' `RemittanceCalculation` types: add

```ts
benchmark?: { rate: number; receive_amount: number } | null;
```

- [ ] **Step 4: Check + commit**

```bash
cd apps/main && bun run check
cd ../tools && bun run check
git add apps/main/ apps/tools/
git commit -m "feat(main,tools): currency converter page + mid-market benchmark display on remittance (§112)"
```

---

### Task 11: Final verification sweep

- [ ] **Step 1: Full backend build + targeted tests**

```bash
make sqlc-gen   # confirm no diff drift
go build ./...
go test ./internal/fx/ ./internal/remittance/ ./internal/features/ ./internal/worker/
go vet ./internal/fx/ ./internal/remittance/
```

Expected: all pass; `features` tests confirm `fx.enabled` registry entry.

- [ ] **Step 2: Route smoke check**

With a local DB migrated (`make migrate` already applied 000777): start `cmd/api`, then:

```bash
curl -s "http://localhost:8080/api/v1/fx/currencies" | head -c 300     # expect {"data":[]} or list once rates exist
curl -s "http://localhost:8080/api/v1/fx/convert?amount=100&from=USD&to=NGN"  # expect 503 fx_unavailable pre-fetch
curl -s -X POST -H "Authorization: Bearer <admin>" -H "content-type: application/json" \
  -d '{"api_config":{"engine":"oxr","app_id":"<KEY>","base_currency":"USD","min_interval_minutes":60,"corridors":["GBP->NGN","USD->NGN"]},"is_active":true,"is_benchmark":true}' \
  "http://localhost:8080/api/v1/admin/remittance/providers/<mid-market-id>"
curl -s -X POST -H "Authorization: Bearer <admin>" "http://localhost:8080/api/v1/admin/remittance/providers/<mid-market-id>/fetch"
curl -s "http://localhost:8080/api/v1/fx/convert?amount=100&from=USD&to=NGN"  # expect converted amount
curl -s "http://localhost:8080/api/v1/remittance/rates?send=USD&receive=NGN"  # expect mid-market benchmark row
curl -s "http://localhost:8080/api/v1/remittance/calculate?send_amount=100&send=USD&receive=NGN" # expect benchmark field separate from results
```

If no OXR key is available locally, verify with a stub: temporarily point `endpoint` at a `httptest`-like local server, or verify the throttle/parse paths via `go test` (already covered) and mark the live fetch as manually verified later.

- [ ] **Step 3: Frontend checks**

```bash
cd apps/tools && bun run check && bun run build
cd ../main && bun run check && bun run build
cd ../admin && bun run check
```

- [ ] **Step 4: Commit remaining + final state**

```bash
git status   # only intentionally-modified files staged across prior commits
git log --oneline -12   # sanity: all task commits on main
```

---

## Self-review notes (for the implementer)

- **Throttle edge**: `FetchRates` returning `nil, nil` while throttled is correct — `FetchProviderRates` logs "no snapshots" and continues; remittance rows from prior fetches stay fresh within the 15-min corridor cache TTL anyway.
- **Cross-matrix ordering**: the `remittance:fetch_rates` sweep and `fx` matrix write happen inside one adapter call — no inter-task ordering issue.
- **`is_public` on fx.subdomain/custom_domain**: verified — `GetPublicSettings` filters `is_public = true`; `remittance.subdomain` is non-public and only works via the `DEFAULT_SUBDOMAINS` fallback in tools `hooks.server.ts`. We seed both fx host keys public AND mark them `Public: true` in `internal/control/settings_catalog.go` so Control Panel upserts preserve the flag.
- **Admin `is_benchmark` seed row**: the seeded `mid-market` row is inactive — admin must PATCH `api_config` + `is_active:true` to activate; document this in the §112 section file ops notes.
