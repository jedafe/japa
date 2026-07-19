# Per-Type Feed Card Style — Backend (Layer 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Follow the `jc-backend-develop` and `jc-backend-migrate` conventions.

**Goal:** Upgrade the tenant `feed.card_style` setting from a single scalar to a `{ default, by_type }` map, exposed by `GET /feed/card-style`, so the feed can pick a card style per content type.

**Architecture:** A pure `resolveCardStyleConfig(raw string) CardStyleConfig` normalizes the JSONB setting value (tolerating the legacy scalar), keeping the parse/validation logic unit-testable and out of the HTTP handler. The existing `GetCardStyle` handler becomes a thin caller that returns the normalized config. A guarded, idempotent migration reshapes the seeded scalar in place. No schema change — `system_settings.value` is already `jsonb`.

**Tech Stack:** Go, chi router, pgx/pgtype, `encoding/json`, golang-migrate (`cmd/migrate`), standard `testing`.

## Global Constraints

- `CardStyle` is exactly one of: `reddit`, `compact`, `grid`, `magazine`. Default is `reddit`.
- Endpoint `GET /api/v1/feed/card-style` stays **public** (no auth) and returns `{ "data": { "default": CardStyle, "by_type": { [content_type]: CardStyle } } }`.
- The reader MUST tolerate the legacy scalar value (`"reddit"`) and normalize it to `{ default: <scalar>, by_type: {} }`.
- No new column / index / table. No `sqlc` regeneration (no query shape change).
- Migration is paired up/down, number `000147`, guarded by `jsonb_typeof`, idempotent.
- Tests: `go test ./...`; pure functions use `t.Parallel()` and table-driven cases (mirror `internal/feed/feed_test.go`).
- Spec: `docs/superpowers/specs/2026-07-19-per-type-feed-card-style-design.md`.

---

### Task 1: Pure card-style config resolver

**Files:**
- Create: `internal/feed/cardstyle.go`
- Test: `internal/feed/cardstyle_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `const CardStyleReddit = "reddit"`, `CardStyleCompact = "compact"`, `CardStyleGrid = "grid"`, `CardStyleMagazine = "magazine"`, `DefaultCardStyle = CardStyleReddit`
  - `type CardStyleConfig struct { Default string ` + "`json:\"default\"`" + `; ByType map[string]string ` + "`json:\"by_type\"`" + ` }`
  - `func resolveCardStyleConfig(raw string) CardStyleConfig` — `ByType` is always non-nil.

- [ ] **Step 1: Write the failing test**

Create `internal/feed/cardstyle_test.go`:

```go
package feed

import (
	"reflect"
	"testing"
)

func TestResolveCardStyleConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want CardStyleConfig
	}{
		{
			name: "empty -> default",
			raw:  "",
			want: CardStyleConfig{Default: "reddit", ByType: map[string]string{}},
		},
		{
			name: "legacy scalar",
			raw:  `"magazine"`,
			want: CardStyleConfig{Default: "magazine", ByType: map[string]string{}},
		},
		{
			name: "legacy invalid scalar coerced to reddit",
			raw:  `"devto"`,
			want: CardStyleConfig{Default: "reddit", ByType: map[string]string{}},
		},
		{
			name: "object with per-type overrides",
			raw:  `{"default":"reddit","by_type":{"blog_post":"grid","discussion":"reddit"}}`,
			want: CardStyleConfig{Default: "reddit", ByType: map[string]string{"blog_post": "grid", "discussion": "reddit"}},
		},
		{
			name: "object invalid default coerced",
			raw:  `{"default":"bogus","by_type":{}}`,
			want: CardStyleConfig{Default: "reddit", ByType: map[string]string{}},
		},
		{
			name: "object drops invalid per-type entry",
			raw:  `{"default":"compact","by_type":{"blog_post":"grid","discussion":"bogus"}}`,
			want: CardStyleConfig{Default: "compact", ByType: map[string]string{"blog_post": "grid"}},
		},
		{
			name: "object missing by_type",
			raw:  `{"default":"grid"}`,
			want: CardStyleConfig{Default: "grid", ByType: map[string]string{}},
		},
		{
			name: "unparseable -> default",
			raw:  `not json`,
			want: CardStyleConfig{Default: "reddit", ByType: map[string]string{}},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolveCardStyleConfig(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("resolveCardStyleConfig(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/feed/ -run TestResolveCardStyleConfig`
Expected: FAIL — build error, `undefined: resolveCardStyleConfig` / `CardStyleConfig`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/feed/cardstyle.go`:

```go
package feed

import "encoding/json"

// Allowed feed card-style values.
const (
	CardStyleReddit   = "reddit"
	CardStyleCompact  = "compact"
	CardStyleGrid     = "grid"
	CardStyleMagazine = "magazine"
)

// DefaultCardStyle applies when no setting is present or a value is invalid.
const DefaultCardStyle = CardStyleReddit

// CardStyleConfig is a tenant's feed card-style configuration: a default
// style plus optional per-content-type overrides.
type CardStyleConfig struct {
	Default string            `json:"default"`
	ByType  map[string]string `json:"by_type"`
}

// validCardStyle reports whether s is one of the four allowed styles.
func validCardStyle(s string) bool {
	switch s {
	case CardStyleReddit, CardStyleCompact, CardStyleGrid, CardStyleMagazine:
		return true
	default:
		return false
	}
}

// resolveCardStyleConfig normalizes the raw JSONB value of the feed.card_style
// setting. It accepts an empty string (no setting), a legacy JSON string
// ("reddit"), or a JSON object {default, by_type}. Any invalid style is
// coerced: an invalid default becomes reddit; an invalid per-type entry is
// dropped. ByType is always non-nil.
func resolveCardStyleConfig(raw string) CardStyleConfig {
	cfg := CardStyleConfig{Default: DefaultCardStyle, ByType: map[string]string{}}

	if raw == "" {
		return cfg
	}

	// Legacy scalar: the value is a JSON string like "reddit".
	var scalar string
	if err := json.Unmarshal([]byte(raw), &scalar); err == nil {
		if validCardStyle(scalar) {
			cfg.Default = scalar
		}
		return cfg
	}

	// Object form: {default, by_type}.
	var obj CardStyleConfig
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return cfg
	}

	if validCardStyle(obj.Default) {
		cfg.Default = obj.Default
	}
	for ct, style := range obj.ByType {
		if validCardStyle(style) {
			cfg.ByType[ct] = style
		}
	}
	return cfg
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/feed/ -run TestResolveCardStyleConfig -v`
Expected: PASS (all eight subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/feed/cardstyle.go internal/feed/cardstyle_test.go
git commit -m "feat(feed): add per-type card-style config resolver

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Serve the map from GET /feed/card-style

**Files:**
- Modify: `internal/feed/handlers.go:261-285` (the `GetCardStyle` method)
- Test: `internal/feed/cardstyle_test.go` (add a handler test)

**Interfaces:**
- Consumes: `resolveCardStyleConfig` and `CardStyleConfig` (Task 1); existing `Handlers` struct with `settings *config.Settings`; `tenant.FromContext`, `tenant.TenantContextKey`, `tenant.Tenant`; `writeJSON`, `writeError`.
- Produces: `GET /api/v1/feed/card-style` → `{ "data": { "default", "by_type" } }`.

- [ ] **Step 1: Write the failing test**

Append to `internal/feed/cardstyle_test.go` (and extend the import block to add `context`, `encoding/json`, `net/http`, `net/http/httptest`, and `github.com/jedafe/japa/internal/tenant`):

```go
func TestGetCardStyleDefaultShape(t *testing.T) {
	t.Parallel()

	// settings is nil, so the handler reads no stored value and must return
	// the normalized default config. This locks the response field names.
	h := &Handlers{}

	req := httptest.NewRequest(http.MethodGet, "/feed/card-style", nil)
	ctx := context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{})
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.GetCardStyle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var body struct {
		Data CardStyleConfig `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Data.Default != "reddit" {
		t.Errorf("data.default = %q, want reddit", body.Data.Default)
	}
	if body.Data.ByType == nil || len(body.Data.ByType) != 0 {
		t.Errorf("data.by_type = %v, want empty non-nil map", body.Data.ByType)
	}
}
```

The final import block of `cardstyle_test.go` should read:

```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/jedafe/japa/internal/tenant"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/feed/ -run TestGetCardStyleDefaultShape`
Expected: FAIL — the current handler returns `{"data":{"card_style":"reddit"}}`, so `data.default` is empty (`""`), tripping the assertion.

- [ ] **Step 3: Rewrite the handler**

Replace the body of `GetCardStyle` in `internal/feed/handlers.go` (lines 261-285) with:

```go
func (h *Handlers) GetCardStyle(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	raw := ""
	if h.settings != nil {
		if val, err := h.settings.GetSetting(r.Context(), t.ID, "feed.card_style"); err == nil {
			raw = val
		}
	}

	cfg := resolveCardStyleConfig(raw)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": cfg,
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/feed/ -run 'TestGetCardStyleDefaultShape|TestResolveCardStyleConfig' -v`
Expected: PASS.

- [ ] **Step 5: Verify the whole package + build still compile**

Run: `go build ./... && go test ./internal/feed/`
Expected: build succeeds; feed tests pass. (Confirms no other caller depended on the removed `card_style` field.)

- [ ] **Step 6: Commit**

```bash
git add internal/feed/handlers.go internal/feed/cardstyle_test.go
git commit -m "feat(feed): GET /feed/card-style returns {default, by_type} map

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Migration — reshape the seeded scalar

**Files:**
- Create: `migrations/000147_feed_card_style_per_type.up.sql`
- Create: `migrations/000147_feed_card_style_per_type.down.sql`

**Interfaces:**
- Consumes: existing `system_settings` row `key='feed.card_style'` seeded as a JSON string in `migrations/000128`.
- Produces: that row reshaped to `{"default": <old>, "by_type": {}}` (jsonb object).

- [ ] **Step 1: Write the up migration**

Create `migrations/000147_feed_card_style_per_type.up.sql`:

```sql
-- Reshape feed.card_style from a scalar string to a {default, by_type} object.
-- Backward-compatible: the Go reader (resolveCardStyleConfig) also tolerates
-- the legacy scalar, so this is data hygiene, not a hard requirement.
-- Guarded by jsonb_typeof and therefore idempotent.
UPDATE system_settings
SET value = jsonb_build_object('default', value, 'by_type', '{}'::jsonb),
    updated_at = now()
WHERE key = 'feed.card_style'
  AND jsonb_typeof(value) = 'string';
```

- [ ] **Step 2: Write the down migration**

Create `migrations/000147_feed_card_style_per_type.down.sql`:

```sql
-- Restore feed.card_style to its scalar form (the default value).
UPDATE system_settings
SET value = value->'default',
    updated_at = now()
WHERE key = 'feed.card_style'
  AND jsonb_typeof(value) = 'object'
  AND value ? 'default';
```

- [ ] **Step 3: Apply the migration**

Run: `make migrate`
Expected: migration `000147` applies with no error.

- [ ] **Step 4: Verify the reshape**

Inspect the row against the same database `make migrate` targets, using the project's DB client (`psql`):

```sql
SELECT value FROM system_settings WHERE key = 'feed.card_style';
```

Expected value: `{"default": "reddit", "by_type": {}}` (was the scalar `"reddit"`).

- [ ] **Step 5: Confirm idempotency**

Re-run: `make migrate`
Expected: no-op (the `jsonb_typeof(value) = 'string'` guard excludes the already-reshaped row); no error.

- [ ] **Step 6: Commit**

```bash
git add migrations/000147_feed_card_style_per_type.up.sql migrations/000147_feed_card_style_per_type.down.sql
git commit -m "feat(feed): migrate feed.card_style scalar to {default, by_type}

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Acceptance (Layer 1 done)

- `GET /feed/card-style` returns `{ "data": { "default": "reddit", "by_type": {} } }` on a fresh tenant.
- Setting `feed.card_style` to `{"default":"reddit","by_type":{"blog_post":"grid"}}` (via SQL or, later, the Control UI) makes the endpoint return that map; invalid styles are coerced/dropped.
- Legacy scalar values still resolve correctly (backward compatibility).
- `go test ./...` is green; `go build ./...` succeeds.
