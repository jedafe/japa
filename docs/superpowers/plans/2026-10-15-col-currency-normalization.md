# Cost-of-Living Currency Normalization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every cost-of-living money field renders in the host place's local currency with a base-currency equivalent — wiki Lagos shows `₦28,600 ($20)`, Accra shows `₵345 ($34)` — while the `/tools/cost-of-living` calculator and JCE matrices normalize all values to the configured base currency (`$20` vs `$34`).

**Architecture:** Three surfaces, one canonical data source. The geo space's `custom_field_values` (what the wiki page renders, including `options.computed` folds) is the single source of truth: the calculator overlays it onto `comparison_values`/`intel_cost_data` in `costofliving.CompareCities`, and JCE hydrates cells from it via the `col_city` item_ref resolver. So normalizing at each read seam covers all three cases without touching stored data. Display/normalization currency is `fx.default_base_currency` (default `USD`) read from `system_settings` — one knob drives all three surfaces. The §58 country→currency linkage (`GetSpaceCountryCurrencyCode`, already implemented) supplies each space's local currency; the §112 FX matrix (Valkey-cached, `fx.Service`) supplies the rate.

**Tech Stack:** Go 1.x backend (chi, sqlc/pgx, pgx, Valkey), SvelteKit 2 + Svelte 5 runes frontends (`apps/main`, `apps/tools`), golang-migrate SQL migrations, vitest for frontend tests.

**Answers to the earlier open questions (locked):**

1. Display/equivalent currency = `fx.default_base_currency` setting — all three surfaces.
2. JCE normalizes to the base currency.
3. Missing FX coverage: values pass through **raw** and flagged (`converted:false` / no equivalent) — never silently dropped, never fake-normalized. Ratios and the overall index are computed over normalized cells only (a ratio across unnormalized currencies is worse than none). If the FX matrix is entirely unavailable or `fx.enabled` is off, every surface degrades to today's behavior.
4. Every money-typed field gets the equivalent: `currency`, `price_range` (each bound), and `price_parts` parts of `kind:"money"`.
5. `qf_currency` auto-populates: new field-options flag `{"default_source":"space_currency"}` on text fields — when the stored cfv is empty, the backend substitutes the resolved currency's display name (e.g. "Nigerian Naira"). Editors can still override per city.
6. Yes — cases 2 and 3 read the same canonical store as case 1 (geo-space cfv + computed folds). The calculator uses it as an overlay over seeded intel data; JCE hydrates from it per item_ref. Nothing in this plan writes to that store.

## Global Constraints

- Rule 16: no hardcoded currency lists/codes on the frontend — codes and rates come from API payloads only.
- `options.currency_code` on a field remains the explicit override and **wins** over context resolution (§58 contract — keep it; this plan only removes it from seeded *local-price* fields where it was wrong).
- Never edit an applied migration; forward-fix with a new one.
- Money fields with a resolved field/local code equal to the base currency render **no** parenthetical (a `$20` doesn't need `($20)`).
- Valkey keys must use `valkey.MustKey` conventions; no new keys are added in this plan (the existing `jc:fx:matrix::*` cache is reused).
- Backend tests: `go test ./internal/<pkg>/...`; frontend tests: `vitest run` inside the app dir; typecheck: `npm run check` (svelte-check) inside the app dir; migrations: `make migrate`; sqlc regen: `make sqlc-gen`.

## Data-flow map (verified)

- Wiki page fields: `wiki.Service.GetWikiPage` → resolves `spaceCurrency` via `store.ResolveSpaceCurrencyCode` (`internal/wiki/service.go:769-776`, `store.go:1179-1211`) → `WikiPageDetailDTO.SpaceCurrencyCode` → `WikiItemTables`/`WikiQuickFacts`/`WikiRollupPanel` → `formatWikiFieldValue`/`formatPriceRange`/`formatPricePart` (`apps/main/src/lib/utils/wikiFieldValue.ts`).
- Entity fields rail: `wiki.Service.GetEntityFields` (`internal/wiki/service_fields.go:484-658`) → `EntityFieldsDTO.EntityCurrencyCode` → `EntityFieldsPanel`/`SuggestEdit`.
- Calculator: `costofliving.Service.CompareCities` (`internal/costofliving/service.go:322+`) — `GetValuesForCities` (persisted/intel) then `ListSpaceFieldValues` cfv overlay per `geo_space_id` → `dimValues` floats → ratios + `overall_index`.
- JCE: `comparison.Service.GetComparisonMatrix` (`internal/comparison/service.go:1555-1678`) — persisted `comparison_values` win per cell; gaps hydrate via `col_city` resolver (`internal/comparison/resolver/city.go`) whose metadata carries `geo_space_id` and `field_values` (cfv incl. computed folds).
- FX: `fx.Service.GetMatrix(ctx, base)` → `fx_rates` latest snapshot, Valkey-cached; cross-rate math `rate = matrix[to]/matrix[from]`; USD-base matrix ⇒ `baseAmount = localAmount / rates[local]`.

---

### Task 1: Strip seeded `currency_code` from wiki CoL field options + `default_source` on `qf_currency`

**Files:**
- Create: `migrations/000869_wiki_col_currency_strip.up.sql`
- Create: `migrations/000869_wiki_col_currency_strip.down.sql`
- Modify: `db/seed/wiki_example_cost_of_living.sql` (remove every `"currency_code":"CAD"` / `"currency_code":"CAD"` inside `price_parts` options objects; add `"default_source":"space_currency"` to `qf_currency` inserts)
- Do NOT modify: `migrations/000859_wiki_city_housing_fields_and_stale_cfv.up.sql` (already applied; the new migration corrects its data)

**Interfaces:**
- Produces: `custom_fields.options` for CoL groups no longer carry `currency_code` ⇒ `fieldCurrencyCode(options)` returns `''` ⇒ `space_currency_code` fallback engages. `qf_currency` fields carry `{"default_source":"space_currency"}` consumed by Task 3.

- [ ] **Step 1: Write the migration**

`migrations/000869_wiki_col_currency_strip.up.sql`:

```sql
-- 000869_wiki_col_currency_strip.up.sql
-- §58 fix: the wiki cost-of-living field schemas were seeded with
-- options.currency_code = "CAD" (db/seed/wiki_example_cost_of_living.sql
-- + 000859 housing fields). A field-declared code wins over the host
-- space's resolved country currency, so every city rendered CAD.
-- These fields hold LOCAL prices — they must not declare a code so the
-- §58 country→currency linkage denominates them per space.
-- qf_currency gains {"default_source":"space_currency"}: empty values
-- auto-render the space's resolved currency name (read-time, overridable).
BEGIN;

UPDATE custom_fields f
SET options = f.options - 'currency_code'
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug IN (
    'wiki_city_cost_of_living', 'wiki_country_cost_of_living',
    'wiki_city_prices', 'wiki_city_housing',
    'wiki_city_prices_food_groceries', 'wiki_city_prices_restaurants',
    'wiki_city_prices_utilities', 'wiki_city_prices_lifestyle',
    'wiki_city_prices_transportation'
  )
  AND f.options ? 'currency_code';

UPDATE custom_fields f
SET options = COALESCE(f.options, '{}'::jsonb) || '{"default_source":"space_currency"}'::jsonb
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug LIKE 'wiki\_%'
  AND f.key = 'qf_currency'
  AND f.field_type = 'text';

COMMIT;
```

`migrations/000869_wiki_col_currency_strip.down.sql`:

```sql
-- 000869_wiki_col_currency_strip.down.sql
-- Reverts only the default_source flag. The currency_code strip is
-- corrective — restoring "CAD" would resurrect the bug it fixed.
BEGIN;
UPDATE custom_fields f
SET options = f.options - 'default_source'
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug LIKE 'wiki\_%'
  AND f.key = 'qf_currency';
COMMIT;
```

- [ ] **Step 2: Update the seed file**

In `db/seed/wiki_example_cost_of_living.sql`:

- Delete every `{"currency_code":"CAD"}` occurrence in INSERT/UPDATE `options` payloads (the `price_range`/`currency` fields).
- In every `price_parts` options object (the transportation block, e.g. `'{"parts":[...],"primary_part":"price","currency_code":"CAD"}'`), remove the `,"currency_code":"CAD"` member only — keep `parts`, `primary_part`, `emphasize`.
- For each `qf_currency` insert row (`options` arg `NULL` today), pass `'{"default_source":"space_currency"}'` instead.
- Keep the `qf_currency` *cfv values* (`'CAD'` text) — they're per-city editorial overrides and still win over the default.

- [ ] **Step 3: Apply and verify**

Run: `make migrate` then:

```sql
SELECT g.slug, f.key, f.options FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id
WHERE g.slug LIKE 'wiki\_%' AND f.options ? 'currency_code';
-- Expected: 0 rows
```

- [ ] **Step 4: Commit**

```bash
git add migrations/000868_* db/seed/wiki_example_cost_of_living.sql
git commit -m "fix(wiki): strip seeded CAD currency_code so spaces resolve their country currency"
```

---

### Task 2: `fx.Service.BaseRate` — the display-rate primitive

**Files:**
- Modify: `internal/fx/service.go` (add method after `Convert`)
- Test: `internal/fx/service_test.go`

**Interfaces:**
- Produces (consumed by Tasks 3, 5, 7):
  ```go
  // BaseRate returns units of `quote` per 1 unit of the configured
  // fx.default_base_currency (e.g. 1430 NGN per 1 USD), plus the base
  // code and matrix freshness. Equivalent-in-base = amount / rate.
  // quote equal to base → rate 1, stale=false. Errors: ErrDisabled,
  // ErrEmptyMatrix, ErrUnsupportedCurrency.
  func (s *Service) BaseRate(ctx context.Context, quote string) (rate float64, base string, fetchedAt time.Time, stale bool, err error)
  ```

- [ ] **Step 1: Write the failing test**

In `internal/fx/service_test.go` — mirror existing table tests for `Convert`/`GetMatrix`; stub the store the same way existing tests do:

```go
func TestBaseRate(t *testing.T) {
	// matrix: USD base, NGN=1430, GHS=10.2
	rate, base, _, stale, err := svc.BaseRate(ctx, "NGN")
	// assert err==nil, base=="USD", rate==1430, stale==false

	rate, _, _, _, err = svc.BaseRate(ctx, "USD")
	// assert rate==1, err==nil

	_, _, _, _, err = svc.BaseRate(ctx, "XXX")
	// assert errors.Is(err, ErrUnsupportedCurrency)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fx/ -run TestBaseRate -v`
Expected: FAIL — `BaseRate` undefined.

- [ ] **Step 3: Implement**

In `internal/fx/service.go`, after `Convert`:

```go
// BaseRate returns units of `quote` per 1 unit of the configured default
// base currency — the read seam for "(≈base)" equivalents and COL/JCE
// normalization (base amount = local amount / rate). The quote equal to
// base resolves to rate 1 without matrix coverage requirements.
func (s *Service) BaseRate(ctx context.Context, quote string) (float64, string, time.Time, bool, error) {
	if !s.IsEnabled(ctx) {
		return 0, "", time.Time{}, false, ErrDisabled
	}
	base := s.DefaultBaseCurrency(ctx)
	quote = strings.ToUpper(quote)

	m, err := s.GetMatrix(ctx, base)
	if err != nil {
		return 0, base, time.Time{}, false, err
	}

	stale := matrixStale(m.FetchedAt, s.staleHours(ctx), time.Now())

	if quote == base {
		return 1, base, m.FetchedAt, stale, nil
	}

	rate, ok := m.Rates[quote]
	if !ok || rate <= 0 {
		return 0, base, m.FetchedAt, stale, fmt.Errorf("%w: %s", ErrUnsupportedCurrency, quote)
	}

	return rate, base, m.FetchedAt, stale, nil
}
```

(`strings` is already imported in service.go.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/fx/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fx/service.go internal/fx/service_test.go
git commit -m "feat(fx): add BaseRate display-rate primitive for local→base equivalents"
```

---

### Task 3: Wiki backend — `currency_fx` DTO + `default_source` substitution

**Files:**
- Modify: `internal/wiki/service.go` (~line 130-145 DTO, ~769 currency resolve, plus `Service` struct + setter)
- Modify: `internal/wiki/service_fields.go` (~line 262 `EntityCurrencyCode` field, ~605-622 value assembly, ~655 `resolveEntityCurrencyCode` call)
- Modify: `internal/wiki/store.go` (add `CurrencyDisplayName`)
- Modify: `internal/customfields/customfields.go` (add `DefaultSourceFromOptions`)
- Modify: `internal/server/server.go` (wire `wikiSvc.SetFxRateProvider(fxService)` next to existing wiki wiring)
- Test: `internal/wiki/service_fields_test.go` or a new `internal/wiki/currency_fx_test.go` — match existing harness style

**Interfaces:**
- Consumes: `fx.Service.BaseRate` (Task 2).
- Produces:
  ```go
  // In internal/wiki/service.go — the page/entity FX display context.
  type CurrencyFxDTO struct {
      Base      string  `json:"base"`       // "USD" — fx.default_base_currency
      Rate      float64 `json:"rate"`       // local units per 1 base unit
      FetchedAt string  `json:"fetched_at"` // RFC3339
      Stale     bool    `json:"stale"`
  }
  // WikiPageDetailDTO gains: SpaceCurrencyFx *CurrencyFxDTO `json:"space_currency_fx,omitempty"`
  // EntityFieldsDTO gains:   EntityCurrencyFx *CurrencyFxDTO `json:"entity_currency_fx,omitempty"`
  // Service gains:  SetFxRateProvider(FxRateProvider)
  // customfields gains:
  func DefaultSourceFromOptions(options []byte) string // {"default_source":"..."} → "...", else ""
  ```

- [ ] **Step 1: Failing test — field default substitution + fx DTO**

New test covering: a text field with `{"default_source":"space_currency"}` and no cfv renders `"Nigerian Naira"` when the entity resolves NGN; a populated cfv wins; a field without the flag is untouched. And: when the fx provider returns NGN→USD rate 1430, the DTO carries `entity_currency_fx{base:"USD", rate:1430}`; provider nil or error → field omitted, no failure.

- [ ] **Step 2: Run to verify fail**

Run: `go test ./internal/wiki/ -run TestCurrencyFx -v`
Expected: FAIL — symbols undefined.

- [ ] **Step 3: Implement**

`internal/customfields/customfields.go` (next to `CurrencyCodeFromOptions`):

```go
// DefaultSourceFromOptions extracts a field's read-time default source
// ({"default_source":"space_currency"}) — currently the only supported
// value is "space_currency": an empty text value renders the entity's
// resolved country currency name instead of blank.
func DefaultSourceFromOptions(options []byte) string {
	if len(options) == 0 {
		return ""
	}
	var o struct {
		DefaultSource string `json:"default_source"`
	}
	if err := json.Unmarshal(options, &o); err != nil {
		return ""
	}
	return o.DefaultSource
}
```

`internal/wiki/store.go`:

```go
// CurrencyDisplayName returns the currencies.name for a code — the
// display value for default_source:"space_currency" text fields.
// Empty string when the code is unknown.
func (s *Store) CurrencyDisplayName(ctx context.Context, code string) string {
	c, err := s.read.GetCurrencyByCode(ctx, code)
	if err != nil || c.Name == "" {
		return ""
	}
	return c.Name
}
```

(`GetCurrencyByCode` already exists in `internal/db/queries/credit_currency.sql`.)

`internal/wiki/service.go` — Service field + setter + helper:

```go
// FxRateProvider is the narrow slice of *fx.Service the wiki read path
// needs — injected via SetFxRateProvider so a missing FX subsystem
// degrades to local-currency-only display.
type FxRateProvider interface {
	BaseRate(ctx context.Context, quote string) (rate float64, base string, fetchedAt time.Time, stale bool, err error)
}

func (s *Service) SetFxRateProvider(p FxRateProvider) { s.fxRates = p }

// currencyFx builds the page/entity FX context for a resolved local code.
// Nil on any failure — display degrades to local-only, never errors.
func (s *Service) currencyFx(ctx context.Context, localCode string) *CurrencyFxDTO {
	if s.fxRates == nil || localCode == "" {
		return nil
	}
	rate, base, fetchedAt, stale, err := s.fxRates.BaseRate(ctx, localCode)
	if err != nil {
		return nil
	}
	return &CurrencyFxDTO{
		Base: base, Rate: rate,
		FetchedAt: fetchedAt.UTC().Format(time.RFC3339), Stale: stale,
	}
}
```

In `GetWikiPage` (right after the `spaceCurrency` resolve at ~line 769), set on every DTO variant that already gets `SpaceCurrencyCode`:

```go
dto.SpaceCurrencyFx = s.currencyFx(ctx, spaceCurrency)
```

`internal/wiki/service_fields.go` — in `GetEntityFields` (~line 655):

```go
dto.EntityCurrencyCode = s.resolveEntityCurrencyCode(ctx, entityType, entityID, isSpaceBackedEntityType(entityType))
dto.EntityCurrencyFx = s.currencyFx(ctx, dto.EntityCurrencyCode)
```

And the `default_source` substitution — a post-pass beside `applyComputedFields`, run per dto set after computed folding, needing the entity currency name resolved once per call. **Two corrections vs the first draft:** iterate by index (struct copies in `range` don't write back) and skip `!dto.Visible` fields (their values were just withheld — don't re-fill them):

```go
// applySpaceCurrencyDefaults fills empty, visible text fields whose
// options carry {"default_source":"space_currency"} with the resolved
// currency's display name — cfv always wins, so only unset fields are
// touched.
func (s *Service) applySpaceCurrencyDefaults(ctx context.Context, dtos []EntityFieldDTO, currencyCode string) {
	if currencyCode == "" {
		return
	}
	name := ""
	resolved := false
	for i := range dtos {
		dto := &dtos[i]
		if dto.FieldType != "text" || dto.Value != nil || !dto.Visible {
			continue
		}
		if customfields.DefaultSourceFromOptions(dto.Options) != "space_currency" {
			continue
		}
		if !resolved {
			name, resolved = s.store.CurrencyDisplayName(ctx, currencyCode), true
		}
		if name != "" {
			v, _ := json.Marshal(name)
			dto.Value = v
		}
	}
}
```

Call sites (both post-pass after `s.applyComputedFields`):
- `GetEntityFields` — service_fields.go ~line 622. `EntityCurrencyCode` is resolved at line 655 *after* the group loop today; hoist the `resolveEntityCurrencyCode` call above the loop into a local `entityCurrency` var, pass it to the post-pass, and assign `dto.EntityCurrencyCode = entityCurrency` afterward (same emitted value).
- `GetPageFields` — service_fields.go ~line 807 (the path feeding `data.page.fields` → `WikiItemTables`). It receives `spaceID` directly — resolve via `s.store.ResolveSpaceCurrencyCode(ctx, spaceID)` once per call.

- [ ] **Step 4: Wire in server.go**

In `internal/server/server.go`, where the wiki service is constructed (search `wiki.NewService` / `SetFeatureChecker` near existing wiki wiring): add `wikiSvc.SetFxRateProvider(fxService)` — `*fx.Service` satisfies the interface once Task 2 lands.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/wiki/ ./internal/customfields/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/wiki/ internal/customfields/ internal/server/server.go
git commit -m "feat(wiki): emit space_currency_fx and auto-default qf_currency from space currency"
```

---

### Task 4: Wiki frontend — `(base)` equivalents on every money field

**Files:**
- Modify: `apps/main/src/lib/utils/wikiFieldValue.ts` (add `fx` param to `formatPriceRange`, `formatPricePart`, `formatPricePartsInline`, `formatWikiFieldValue`)
- Modify: `apps/main/src/lib/api/wiki.ts` (`space_currency_fx` on the page DTO type, `entity_currency_fx` on the fields DTO)
- Modify: `apps/main/src/lib/components/wiki/WikiItemTables.svelte` (new `contextFx` prop; pass to `avgOf`/`rangeOf`/`partCell`/`formatWikiFieldValue`)
- Modify: `apps/main/src/lib/components/wiki/WikiQuickFacts.svelte` (same)
- Modify: `apps/main/src/lib/components/wiki/WikiRollupPanel.svelte` (same — `moneyNum`/`cellValue`/`metricHeadline`)
- Modify: `apps/main/src/lib/components/wiki/EntityFieldsPanel.svelte` (pass `schema.entity_currency_fx`)
- Modify: `apps/main/src/lib/components/wiki/SuggestEdit.svelte` (same two call sites: ~367, ~554)
- Modify: `apps/main/src/routes/(app)/wiki/[space_slug]/[page_slug]/+page.svelte` (pass `data.page.space_currency_fx` as `contextFx`)
- Test: `apps/main/src/lib/utils/wikiFieldValue.test.ts`

**Interfaces:**
- Consumes: `space_currency_fx`/`entity_currency_fx` from Task 3.
- Produces:
  ```ts
  // Rate context for "(base)" equivalents — null/undefined = local only.
  export interface FxEquiv { base: string; rate: number }
  // money(n) → "₦28,600 ($20)"; equivalent omitted when code===base,
  // rate<=0, or fx absent.
  ```

- [ ] **Step 1: Failing tests**

In `wikiFieldValue.test.ts`:

```ts
describe('fx equivalents', () => {
	const fx = { base: 'USD', rate: 1430 }; // 1430 NGN per USD
	const currencies = [
		{ code: 'NGN', name: 'Nigerian Naira', symbol: '₦' },
		{ code: 'USD', name: 'US Dollar', symbol: '$' },
	];
	it('appends base equivalent for price_range', () => {
		expect(formatPriceRange({ avg: 28600 }, 'NGN', currencies, fx)).toBe('₦28,600 ($20)');
	});
	it('equivalents per bound for ranges', () => {
		expect(formatPriceRange({ min: 1430, max: 2860 }, 'NGN', currencies, fx))
			.toBe('₦1,430 ($1) – ₦2,860 ($2)');
	});
	it('omits equivalent when field is already base', () => {
		expect(formatPriceRange({ avg: 20 }, 'USD', currencies, fx)).toBe('$20');
	});
	it('no fx → unchanged output', () => {
		expect(formatPriceRange({ avg: 28600 }, 'NGN', currencies)).toBe('₦28,600');
	});
});
```

- [ ] **Step 2: Run to verify fail**

Run: `cd apps/main && npx vitest run src/lib/utils/wikiFieldValue.test.ts`
Expected: FAIL — 4th arg unsupported.

- [ ] **Step 3: Implement**

In `wikiFieldValue.ts`, add the shared equivalent helper and a trailing optional `fx` param on the four formatters:

```ts
export interface FxEquiv {
	base: string;
	/** Local units per 1 base unit. */
	rate: number;
}

/** "(…)" equivalent in the display base currency — '' when the code IS
 *  the base, the rate is unusable, or fx is absent. */
function equivSuffix(n: number, code: string, fx: FxEquiv | undefined, currencies?: CurrencyInfo[] | null): string {
	if (!fx || !code || code === fx.base || !(fx.rate > 0)) return '';
	const sym = currencySymbol(fx.base, currencies);
	const amt = n / fx.rate;
	const inner = sym && sym !== fx.base ? `${sym}${fmtNum(amt)}` : `${fx.base} ${fmtNum(amt)}`;
	return ` (${inner})`;
}
```

Then thread `fx?: FxEquiv` into `money()` inside `formatPriceRange` (`money = n => … + equivSuffix(n, code, fx, currencies)`), `formatPricePart` (money kind only), `formatPricePartsInline`, and `formatWikiFieldValue` (the `currency`/`price_range`/`price_parts` branches). Keep `fx` the last optional parameter so existing call sites compile unchanged.

In the components: add `contextFx?: FxEquiv` prop mirroring `contextCurrency`; pass through where each formatter is called. In `WikiRollupPanel`, `fieldCode` stays unchanged (descendant rows share the host country's currency in practice — the same-city-equivalent caveat is acceptable and documented); apply `equivSuffix` inside `moneyNum`.

- [ ] **Step 4: Run tests + typecheck**

Run: `cd apps/main && npx vitest run src/lib/utils/wikiFieldValue.test.ts && npm run check`
Expected: PASS, no new svelte-check errors.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/utils/wikiFieldValue.* apps/main/src/lib/components/wiki/ apps/main/src/lib/api/wiki.ts "apps/main/src/routes/(app)/wiki/[space_slug]/[page_slug]/+page.svelte"
git commit -m "feat(wiki): render base-currency equivalents on money fields"
```

---

### Task 5: Calculator backend — normalize matrix to base currency

**Files:**
- Modify: `internal/costofliving/store.go` (add `SpaceCurrencyCodes`)
- Modify: `internal/costofliving/service.go` (`CompareCities` normalization + DTO fields + `SetFxRateProvider`)
- Modify: `internal/server/server.go` (wire provider into the costofliving service)
- Test: `internal/costofliving/service_test.go` and/or `handlers_test.go`

**Interfaces:**
- Consumes: `fx.Service.BaseRate` (Task 2) — same `FxRateProvider` shape as wiki (redeclare locally per Go consumer-interface convention).
- Produces:
  ```go
  // ComparisonMatrix gains:
  DisplayCurrency string `json:"display_currency"` // "USD", "" when fx unavailable
  FxFetchedAt     string `json:"fx_fetched_at,omitempty"`
  FxStale         bool   `json:"fx_stale,omitempty"`
  // CityData gains:
  CurrencyCode string `json:"currency_code,omitempty"` // city's resolved local code
  Converted    bool   `json:"converted"`               // values[] are base-denominated
  // store:
  func (s *Store) SpaceCurrencyCodes(ctx context.Context, ids []pgtype.UUID) map[pgtype.UUID]string
  ```

- [ ] **Step 1: Failing test**

`service_test.go`: fake `FxRateProvider` returning NGN@1430/GHS@10.2; cities Lagos (NGN space) + Accra (GHS space) + Lagos-baseline. Assert `values[city]` = raw/local rate (₦28,600 → 20.0), `display_currency=="USD"`, Lagos `currency_code=="NGN"`, `converted==true`; a city with unresolvable currency keeps raw value, `converted:false`, and is excluded from `ratios`/`overall_index`; provider nil → all `converted:false`, `display_currency==""`, values unchanged (legacy).

- [ ] **Step 2: Run to verify fail**

Run: `go test ./internal/costofliving/ -run TestCompareNormalizes -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

`store.go`:

```go
// SpaceCurrencyCodes resolves each geo space's country currency (§58) —
// one ancestor walk per space; the calculator never exceeds maxCities+1
// spaces so no batch query is needed. Misses are simply absent.
func (s *Store) SpaceCurrencyCodes(ctx context.Context, ids []pgtype.UUID) map[pgtype.UUID]string {
	out := make(map[pgtype.UUID]string, len(ids))
	for _, id := range ids {
		if code, err := s.read.GetSpaceCountryCurrencyCode(ctx, id); err == nil && code != nil && *code != "" {
			out[id] = *code
			continue
		}
		// non-geo fallback: geo_code → country space's currency_code
		// (same chain as wiki.Store.ResolveSpaceCurrencyCode)
		space, err := s.read.GetSpaceByID(ctx, id)
		if err != nil || space.GeoCode == nil || *space.GeoCode == "" {
			continue
		}
		country, err := s.read.GetSpaceByGeoCode(ctx, *space.GeoCode)
		if err == nil && country.CurrencyCode != nil && *country.CurrencyCode != "" {
			out[id] = *country.CurrencyCode
		}
	}
	return out
}
```

`service.go` — **scope normalization to currency dimensions only.** `dimValues` also carries non-money dims (ratings, population); dividing those by FX rates corrupts them. `GetCOLValuesForCitiesRow.DimensionType` is already on the row — add `Money bool` to `dimMeta` set from `v.DimensionType == dimensionTypeCurrency` at line ~431, and declare locally (do NOT import `internal/comparison` — `comparison/resolver` already imports `costofliving`, so that's a cycle):

```go
// dimensionTypeCurrency mirrors comparison.DimensionTypeCurrency —
// declared locally because comparison/resolver imports this package.
const dimensionTypeCurrency = "currency"
```

Then after `dimValues` is fully built (stored values + cfv overlay — both local-denominated per §58), normalize only money dims:

```go
// §58+§112 normalization: every city's money values are denominated in
// its country's currency — convert to the fx display base so ratios and
// the index compare like units. Cities without a resolvable currency or
// matrix coverage keep raw local values flagged converted:false.
ratesByCity, baseCode, fxFetchedAt, fxStale := s.cityBaseRates(ctx, cityOrder, geoSpaceByCity)
if baseCode != "" {
	for dimID, byCity := range dimValues {
		if !dimMetaMap[dimID].Money {
			continue
		}
		for cityKey, v := range byCity {
			if rate, ok := ratesByCity[cityKey]; ok && rate > 0 {
				byCity[cityKey] = v / rate
			}
		}
	}
}
```

where `cityBaseRates` maps `geoSpaceID → code → BaseRate` (dedupe codes so Lagos+Kano share one NGN lookup; nil/absent → no rate → unconverted). Guard the ratio/index computation to only include cities whose values were converted **or** all cities when the provider is absent (legacy raw mode — consistent units-by-accident behavior is better than empty). Emit `display_currency`/`fx_*`/`currency_code`/`converted` on the response DTO.

Degradation contract: `s.fxRates == nil` OR `BaseRate` returns `ErrDisabled`/`ErrEmptyMatrix` on first call → skip normalization wholesale (`display_currency=""`). Per-currency `ErrUnsupportedCurrency` → that city alone stays raw.

- [ ] **Step 4: Wire + test**

`server.go`: `colService.SetFxRateProvider(fxService)` beside existing wiring.

Run: `go test ./internal/costofliving/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/costofliving/ internal/server/server.go
git commit -m "feat(col): normalize calculator matrix values to fx base currency"
```

---

### Task 6: Calculator frontend — base-currency display + staleness

**Files:**
- Modify: `apps/tools/src/lib/types/index.ts` (~line 288-310: `ColCityData` + `ColComparisonMatrix` gains)
- Modify: `apps/tools/src/routes/cost-of-living/+page.svelte` (format cells with `display_currency`; stale chip; unconverted cells render `local code + raw`)
- Test: none today for this page — add `apps/tools/src/lib/utils/` unit test only if a formatter helper is extracted; otherwise verify via `npm run check`

**Interfaces:**
- Consumes: Task 5's DTO fields.

- [ ] **Step 1: Update types**

```ts
export interface ColCityData {
	// ...existing fields
	currency_code?: string;
	converted: boolean;
}
export interface ColComparisonMatrix {
	// ...existing fields
	display_currency: string;
	fx_fetched_at?: string;
	fx_stale?: boolean;
}
```

- [ ] **Step 2: Render**

In `+page.svelte`, the category table cell (`cat.values[city.id]?.toFixed(0)`) becomes: if `matrix.display_currency` && `city.converted` → `formatMoney(value, matrix.display_currency, data.currencies)` (tools app already has `$lib/utils/currency` — used by `currency-converter/+page.svelte`); if not converted → `{value} {city.currency_code}` (or bare when no code). Add a stale-rates note under the Overall Cost Index when `matrix.fx_stale` (reuse the existing stale-warning banner pattern at ~line 323-333, with `t('col.fx_stale', 'Exchange rates may be outdated.')`). Add i18n keys to `apps/tools/src/lib/i18n/en.json` + `fr.json`.

- [ ] **Step 3: Typecheck**

Run: `cd apps/tools && npm run check`
Expected: no new errors.

- [ ] **Step 4: Commit**

```bash
git add apps/tools/src/lib/types/index.ts "apps/tools/src/routes/cost-of-living/+page.svelte" apps/tools/src/lib/i18n/
git commit -m "feat(tools): render calculator values in display currency with stale-rate hint"
```

---

### Task 7: JCE backend — normalize hydrated money cells to base currency

**Files:**
- Modify: `internal/comparison/service.go` (`GetComparisonMatrix` + `hydrateMatrixCell` path, DTO, setters)
- Modify: `internal/server/server.go` (wire `comparisonSvc.SetCurrencyResolver(...)` + `SetFxRateProvider(...)` — `spaces.Store`/`wiki.Store` `ResolveSpaceCurrencyCode` already satisfies the resolver interface; pick whichever is already in scope)
- Test: `internal/comparison/service_test.go` (or the existing matrix test file)

**Interfaces:**
- Consumes: `fx.Service.BaseRate` (Task 2); `geo_space_id` already in `ResolvedItem.Metadata` (`resolver/city.go:177`).
- Produces:
  ```go
  // ComparisonMatrixDimension gains:
  Normalized bool `json:"normalized,omitempty"` // value_rating is base-denominated
  // Service gains:
  type SpaceCurrencyResolver interface {
      ResolveSpaceCurrencyCode(ctx context.Context, spaceID pgtype.UUID) (string, error)
  }
  func (s *Service) SetCurrencyResolver(r SpaceCurrencyResolver)
  func (s *Service) SetFxRateProvider(p FxRateProvider) // same shape as Task 3
  ```

- [ ] **Step 1: Failing test**

`service_test.go`: comparison with a `col_city` item whose resolved metadata has `geo_space_id` (NGN space), a currency dimension hydrated from `field_values` (`{"avg":28600}` → hydrateMatrixCell → `matrixCellNumber` → 28600), fake resolver→NGN, fake fx→1430. Assert `ValueRating==20`, `CurrencyCode=="USD"`, `Normalized==true`. Provider absent → `CurrencyCode=="NGN"`, `Normalized` unset, raw value.

- [ ] **Step 2: Run to verify fail**

Run: `go test ./internal/comparison/ -run TestMatrixNormalizes -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `GetComparisonMatrix`: precompute per-item context once per item (not per dimension):

```go
// §58+§112: resolve the item's currency once — geo-space-backed items
// (col_city) carry geo_space_id in resolved metadata.
type itemFx struct {
	localCode string
	rate      float64 // local units per base; 0 = unconvertible
	base      string
}
```

For each item, when `s.currencyResolver != nil` and resolved metadata has `geo_space_id`: `code = resolver.ResolveSpaceCurrencyCode(...)`; when `s.fxRates != nil`: `rate, base, _, _, err = fxRates.BaseRate(ctx, code)` (cache per code — multiple items share currencies; a `map[string]itemFx` keyed by code avoids duplicate lookups and mirrors the `resolvedItems` cache pattern). Then in the cell fill:

- **Hydrated cells** (`hydrateMatrixCell` success on a money dimension — `DimensionTypeCurrency`): if `itemFx.rate > 0` → `*md.ValueRating /= rate`, `md.CurrencyCode = base`, `md.Normalized = true`; else `md.CurrencyCode = itemFx.localCode`.
- **Persisted cells** (valueIndex hit on `DimensionTypeCurrency`): same treatment — persisted cost_of_living values are local-denominated like cfv (see audit note below).
- Non-`col_city` items (universities, etc.): unchanged — `geo_space_id` absent means no resolver hit, `CurrencyCode` falls back to `dim.SourceFieldOptions` as today.

Keep `hydrateMatrixCell` pure — do the conversion in the caller loop after it returns (it sets `md.ValueRating`; convert after the call inside `GetComparisonMatrix` where `itemFx` is in scope).

- [ ] **Step 4: Wire + test**

`server.go`: wire both setters where `comparison.NewService`/`NewCityResolver` are constructed.

Run: `go test ./internal/comparison/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/comparison/ internal/server/server.go
git commit -m "feat(jce): normalize hydrated cost-of-living cells to fx base currency"
```

---

### Task 8: JCE frontend — currency-aware matrix cells

**Files:**
- Modify: `apps/main/src/lib/types/jce.ts` (`MatrixDimension` gains `currency_code?: string`, `normalized?: boolean` — verify current fields)
- Modify: `apps/main/src/routes/(app)/compare/[slug]/+page.svelte` (`formatValue`)
- Test: `apps/main` — extend an existing comparison util test if one exists, else rely on `npm run check`

**Interfaces:**
- Consumes: Task 7's DTO.

- [ ] **Step 1: Render**

In `formatValue` (`+page.svelte` ~line 156): when `cell.currency_code` is set and `value_rating` is a number → `formatMoney(cell.value_rating, cell.currency_code, page.data.currencies)`; otherwise existing behavior. (`page.data.currencies` is already loaded by the root layout.)

```ts
function formatValue(cell?: MatrixDimension) {
	if (!cell) return '—';
	if (
		cell.currency_code &&
		typeof cell.value_rating === 'number'
	) {
		return formatMoney(cell.value_rating, cell.currency_code, page.data.currencies);
	}
	if (cell.value !== undefined && cell.value !== null) return cell.value;
	// ...unchanged
}
```

- [ ] **Step 2: Typecheck**

Run: `cd apps/main && npm run check`
Expected: no new errors.

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/types/jce.ts "apps/main/src/routes/(app)/compare/[slug]/+page.svelte"
git commit -m "feat(compare): render currency dimension cells with currency_code"
```

---

### Task 9: Data audit + catalogs

**Files:**
- Modify: `.jeos/conventions/api-routes-catalog.md` if DTO fields require documenting per convention (check existing entries for the wiki page DTO / compare endpoint)
- Modify: `.jeos/sections/80-destination-wiki.md` and/or `72-cost-of-living.md` — the project's own spec docs record behavior amendments (match existing amendment style)

- [ ] **Step 1: Audit persisted denominations**

Run against the dev DB:

```sql
-- Sanity-check that stored values are local-denominated:
-- Lagos rent_1br should read ~5–7 figures (NGN), Toronto ~1800–2400 (CAD).
SELECT i.title, i.intel_cost_data->>'rent_1br_city_centre' AS rent_cc,
       i.intel_cost_data->>'groceries_monthly' AS groceries
FROM comparison_items i
JOIN comparison_type_definitions t ON t.id = i.type_id
WHERE t.slug = 'cost_of_living' AND i.deleted_at IS NULL
ORDER BY i.title LIMIT 20;
```

If any city's stored values are clearly USD-denominated (e.g. Lagos rent ≈ 800), decide per-city: correct the data to local, or flag for editorial. Do NOT add code paths for mixed denominations — the cfv overlay already wins per key, so the failure surface is only cells with no wiki coverage.

- [ ] **Step 2: Update the spec/roadmap docs** to record: seeded field options no longer declare currency_code; `default_source:"space_currency"` exists; `space_currency_fx`/`entity_currency_fx`, `display_currency`, `normalized` DTO fields; the local→base normalization contract.

- [ ] **Step 3: Commit**

```bash
git add .jeos/
git commit -m "docs: record col currency normalization and default_source flag"
```

---

## Review amendments (verified against code before execution)

1. **Calculator normalization scope** — `dimValues` also carries non-currency dims; `v.DimensionType` on `GetCOLValuesForCitiesRow` is the gate (`"currency"` only). `internal/comparison` is NOT importable from `costofliving` (cycle via `comparison/resolver`) — use a local const. Seeded CoL dims confirmed `dimension_type='currency'` (archive `000364_seed_cost_of_living_baseline.up.sql`).
2. **`SpaceCurrencyFx` assignment sites** — `service.go:798` (`lazy.SpaceCurrencyCode = spaceCurrency`, covers both lazy template ~1019 and lazy category ~1103 paths) and the literal at ~810.
3. **`applySpaceCurrencyDefaults`** — index iteration + `dto.Visible` guard (see Task 3). Substitution sites: `GetPageFields` ~807 (resolves via `s.store.ResolveSpaceCurrencyCode(ctx, spaceID)`) and `GetEntityFields` ~622 (hoist `resolveEntityCurrencyCode`).
4. **`WikiQuickFacts`** self-fetches `EntityFieldsDTO` (`entityType="space"`, rendered from `WikiSidebar` on both space and page views) — it consumes `entity_currency_fx`, not a prop from the page DTO.
5. **Tools currencies** — `apps/tools` root `+layout.server.ts` already provides `data.currencies`; `formatMoney` is available (used by `currency-converter/+page.svelte`).
6. **`CityData.Values`** (raw strings map) stays un-normalized — informational only; the frontend renders `cat.values`.

## Self-review notes

- **Spec coverage:** case 1 → Tasks 1+3+4; case 2 → Tasks 1+5+6; case 3 → Tasks 1+7+8; `qf_currency` auto-populate → Tasks 1+3; configurable base → Task 2 (`DefaultBaseCurrency` inside `BaseRate`, no callers hardcode USD); degradation policy → Tasks 3/5/7.
- **Seam consistency:** `FxRateProvider`/`SpaceCurrencyResolver` are consumer-declared interfaces (matches existing `serviceAPI`/`FeatureChecker` style in `internal/fx/handlers.go`, `internal/costofliving/service.go:118`).
- **Known risk:** Task 7's "persisted cells are local-denominated" assumption — gated by the Task 9 audit; if false, the fix is data-level (cfv overlay wins anyway).
- **Edge:** `fx.default_base_currency` changed from USD → the parenthetical symbol and `display_currency` follow automatically; seeded `qf_currency` cfv values ('CAD' text on Toronto) are legitimate per-city overrides and stay.
