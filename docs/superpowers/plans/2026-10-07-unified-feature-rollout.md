# Unified Feature & Extension Rollout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `feature_flags` the single source of truth for all extensions/features — toggleable, phased (`planned → in_progress → testing → shipped → archived`), per-surface visible/hidden/disabled with audience and geo scoping — managed from `/control/extensions`, with public roadmap item status derived from the linked flag's phase.

**Architecture:** Approach A from `docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md` (READ IT FIRST — it is the contract). Extend `FlagConfig` jsonb (no new tables); `features.Registry` absorbs `knownExtensions`; a new `SyncFlagPhase` hook keeps linked roadmap items in sync; per-surface enforcement rides the global `geoip.Middleware` GeoContext + a new `X-JC-Surface` header + the caller's RBAC context.

**Tech Stack:** Go 1.2x + chi + sqlc/pgx (backend); SvelteKit 2 + Svelte 5 runes + Tailwind v4 (frontends); bun for all JS tooling (Rule 18); golang-migrate for migrations.

## Global Constraints

- **Rule 2:** never hand-edit `internal/db/gen/*` — edit `internal/db/queries/*.sql`, run `make sqlc-gen`.
- **Rule 16:** all frontend data from API — no hardcoded flag lists in Svelte.
- **Rule 17:** no AI attribution anywhere.
- **ADR-042:** disabled features return 404, never 403.
- JSON field names in `feature_flags.config`: `name`, `description`, `environment`, `rolloutPercentage`, `phase`, `showOnRoadmap`, `surfaces` (map of `{state, audience[], geo_allow[], geo_deny[]}` — snake_case inside `SurfaceRule`, camelCase on `FlagConfig` as spec'd).
- Audience entries are `group:<slug>` or `perm:<node>` prefixed strings.
- `roadmap_items.status` CHECK must include `testing` after migration `000807`.
- Commits go to `main`. Tests: `go test ./internal/...`; frontend: `bun run check` / `bunx svelte-check` per app.
- `features` package must NOT import `internal/rbac` or `internal/auth` in a way that creates a cycle — inject interfaces (the `literalResolverAdapter` pattern at `internal/server/server.go:7990` is the precedent).

---

### Task 1: Migration `000807_unified_rollout` — `testing` status + settings→flags backfill

**Files:**
- Create: `migrations/000807_unified_rollout.up.sql`
- Create: `migrations/000807_unified_rollout.down.sql`

**Interfaces:**
- Produces: `roadmap_items.status` accepts `'testing'`; `feature_flags` rows exist for every gated-but-unseeded key.

- [ ] **Step 1: Write the up migration**

```sql
-- 000807_unified_rollout.up.sql
-- Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §3.3, §7

-- 1) Roadmap status gains 'testing' (§113 lifecycle extension).
ALTER TABLE roadmap_items DROP CONSTRAINT IF EXISTS roadmap_items_status_check;
ALTER TABLE roadmap_items ADD CONSTRAINT roadmap_items_status_check
  CHECK (status IN ('pending','under_review','planned','in_progress',
                    'testing','shipped','declined','archived'));

-- 2) Backfill feature_flags from system_settings bool keys so services
--    migrating off GetBoolOr("X.enabled") keep the operator's choice.
--    EXPLICIT key list — do NOT use a regex: keys like
--    'gamification.badges_enabled', 'ai.cache_enabled',
--    'roadmap.submissions_enabled', 'assessment.ai_roadmap_enabled' are
--    settings (quotas/switches), not feature-reachability toggles, and a
--    pattern match would mint dozens of bogus flags.
--    The list below is the same-name subset of the Task-13 migration map;
--    extend it if Task 13 uncovers more reachability keys.
--    value is jsonb; `value #>> '{}'` unwraps '"true"' and true alike.
INSERT INTO feature_flags (key, enabled, updated_at)
SELECT s.key, COALESCE((s.value #>> '{}')::boolean, true), now()
FROM system_settings s
WHERE s.key = ANY(ARRAY[
  'travel.enabled','story.enabled','wiki.enabled','checklist.enabled',
  'assessment.enabled','remittance.enabled','jobs.enabled','fx.enabled',
  'visa_routes.enabled','scam_alert.enabled','planning_group.enabled',
  'digest.enabled','mutual_aid.enabled','calendar.enabled',
  'japa_stage.enabled','achievement.enabled','service_directory.enabled',
  'scholarship_finder.enabled','arrival_checklist.enabled',
  'appointment_alert.enabled','attestation_guide.enabled',
  'campaign.enabled','col_calculator.enabled','community_locator.enabled',
  'cv_formatter.enabled','housing_guide.enabled','intel.enabled',
  'language_test_hub.enabled','newsletter.enabled','participation.enabled',
  'referral.enabled'
]::text[])
  AND jsonb_typeof(s.value) IN ('string','boolean')
  AND NOT EXISTS (SELECT 1 FROM feature_flags f WHERE f.key = s.key)
ON CONFLICT (key) DO NOTHING;

-- 3) Legacy extension setting keys → their feature_flags equivalents
--    (knownExtensions SettingKey → FeatureFlagKey mapping).
INSERT INTO feature_flags (key, enabled, updated_at)
SELECT v.flag_key, COALESCE((s.value #>> '{}')::boolean, true), now()
FROM (VALUES
  ('extensions.blog.enabled',      'feature.blog'),
  ('extensions.webhooks.enabled',  'webhooks.enabled'),
  ('extensions.ai.enabled',        'feature.ai_content_gen'),
  ('extensions.affiliate.enabled', 'feature.affiliate_program')
) AS v(setting_key, flag_key)
JOIN system_settings s ON s.key = v.setting_key
WHERE jsonb_typeof(s.value) IN ('string','boolean')
ON CONFLICT (key) DO NOTHING;
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000807_unified_rollout.down.sql
-- Revert 'testing' from the roadmap status enum. Fail loudly if rows use it.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM roadmap_items WHERE status = 'testing') THEN
    RAISE EXCEPTION 'cannot revert: roadmap_items rows with status=testing exist';
  END IF;
END $$;

ALTER TABLE roadmap_items DROP CONSTRAINT IF EXISTS roadmap_items_status_check;
ALTER TABLE roadmap_items ADD CONSTRAINT roadmap_items_status_check
  CHECK (status IN ('pending','under_review','planned','in_progress',
                    'shipped','declined','archived'));

-- The feature_flags backfill rows are intentionally NOT removed on down —
-- they represent real operator intent and removing them could re-enable
-- features that were deliberately off.
```

- [ ] **Step 3: Apply and verify**

Run: `make migrate-up` (or the project's migration target), then:
```bash
psql "$DATABASE_URL" -c "INSERT INTO roadmap_items(title,slug,status) VALUES ('t','t','testing'); DELETE FROM roadmap_items WHERE slug='t';"
```
Expected: insert succeeds, delete succeeds.

- [ ] **Step 4: Commit**

```bash
git add migrations/000807_unified_rollout.*.sql
git commit -m "migrate: roadmap testing status + settings->flags backfill (000806)"
```

---

### Task 2: Extend `features.Feature` — absorb `knownExtensions`, register missing flags

**Files:**
- Modify: `internal/features/registry.go`
- Test: `internal/features/features_test.go`

**Interfaces:**
- Produces (used by Tasks 4, 9, 10):
```go
type Feature struct {
	Key              string
	Label            string
	Description      string
	Default          bool
	Category         string            // Content|Social|Commerce|Tools|AI|Engagement|Integrations|Platform
	SettingsCategory string            // control /settings?category=… link; "" = none
	DefaultSurfaces  map[string]string // surface -> "visible"|"hidden"|"disabled"; absent = all visible
}
func RegistryLookup(key string) (Feature, bool) // export the existing unexported registryLookup
```

- [ ] **Step 1: Extend the struct + add fields**

In `internal/features/registry.go`, extend `Feature` per the interface above, then rename `registryLookup` → `RegistryLookup` (exported; update the one caller).

- [ ] **Step 2: Port every `knownExtensions` entry into `Registry`**

Each of the ~28 entries in `internal/control/handlers.go` `knownExtensions` (lines ~3367–3410) becomes a `Feature` carrying `Description`, `Category`, `SettingsCategory` from the catalog row. The extension's `Key`/`SettingKey`/`Version` fields are dropped — `Key` becomes the FeatureFlagKey itself (`feature.blog`, `rag.enabled`, …). Do NOT delete `knownExtensions` yet — Task 9 removes it after `ListExtensions` switches over.

- [ ] **Step 3: Register every gated-but-missing flag**

Add Registry entries for all keys used by `features.Gate` in `internal/server/server.go` that lack one: `roadmap.enabled` (already present — add metadata), `mentorship.enabled`, `mentorship.comparison`, `feature.dm`, `feature.follow`, `feature.forum`, `feature.reactions`, `feature.bookmarks`, `feature.ratings_reviews`, `feature.comparison`, `feature.academy`, `feature.tickets`, `feature.notifications`, `feature.wallet`, `feature.withdraw`, `feature.ai_agent`, `feature.jce`, `feature.jce.nested_items`, `feature.jce.ai_quick`, `feature.jce.data_sources`, `feature.jce.refresh`, `story.enabled`, `wiki.enabled`, `wiki.structured_edits`, `scam_alert.enabled`, `planning_group.enabled`, `digest.enabled`, `mutual_aid.enabled`, `calendar.enabled`, `japa_stage.enabled`, `forum.ama.enabled`, `achievement.enabled`, `service_directory.enabled`, `service_packages.enabled`, `service_coupons.enabled`, `service_packages.tiers_enabled`, `crs_calculator.enabled`, `pof_calculator.enabled`, `eca_guide.enabled`, `sop_generator.enabled`.

Verify none are missed:
```bash
grep -oE 'features\.Gate\(featureChecker, "[^"]+"' internal/server/server.go \
  | sed 's/.*"//;s/"$//' | sort -u > /tmp/gated.txt
grep -oE 'Key: "[^"]+"' internal/features/registry.go | sed 's/.*"//;s/"$//' | sort -u > /tmp/registered.txt
comm -23 /tmp/gated.txt /tmp/registered.txt   # must print nothing
```

- [ ] **Step 4: Add a coverage test so this can never drift again**

In `internal/features/features_test.go`:

```go
func TestRegistryCoversAllGatedKeys(t *testing.T) {
	// Parses server.go for features.Gate(featureChecker, "key") literals and
	// asserts each key exists in Registry. Uses go/parser on the real file —
	// relative path from package dir: ../server/server.go.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "../server/server.go", nil, 0)
	if err != nil { t.Fatalf("parse server.go: %v", err) }
	var gated []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr); if !ok { return true }
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Gate" { return true }
		if len(call.Args) < 2 { return true }
		if lit, ok := call.Args[1].(*ast.BasicLit); ok {
			gated = append(gated, strings.Trim(lit.Value, `"`))
		}
		return true
	})
	for _, key := range gated {
		if _, ok := RegistryLookup(key); !ok {
			t.Errorf("gated flag %q missing from Registry", key)
		}
	}
}
```

- [ ] **Step 5: `go test ./internal/features/…` and commit**

---

### Task 3: `FlagConfig` extension — phase, surfaces, audience, geo + validation

**Files:**
- Modify: `internal/features/features.go` (`FlagConfig` at ~line 411)
- Test: `internal/features/features_test.go`

**Interfaces:**
- Produces:
```go
type SurfaceRule struct {
	State    string   `json:"state"`               // visible|hidden|disabled
	Audience []string `json:"audience,omitempty"`  // group:<slug>|perm:<node>
	GeoAllow []string `json:"geo_allow,omitempty"` // ISO alpha-2, uppercase
	GeoDeny  []string `json:"geo_deny,omitempty"`
}
// added to FlagConfig:
Phase         string                `json:"phase"`
ShowOnRoadmap bool                  `json:"showOnRoadmap"`
Surfaces      map[string]SurfaceRule `json:"surfaces"`

var Phases = []string{"planned","in_progress","testing","shipped","archived"}
var Surfaces = []string{"main","tools","academy","docs","mobile","admin","extension","desktop"}
func ValidPhase(string) bool
func ValidSurface(string) bool
func ValidateConfig(cfg FlagConfig) error // errpkg-wrapped: invalid_phase, invalid_surface, invalid_surface_state, invalid_audience, invalid_geo
```

- [ ] **Step 1: Failing tests** — table test for `ValidateConfig`: valid empty config; bad phase; unknown surface key; bad state; audience entry without prefix; lowercase geo code rejected (or auto-uppercased — pick reject, simpler); `geo_deny` + `geo_allow` both containing `US` is *valid* (deny wins at eval).
- [ ] **Step 2: Implement** the types, constants, `ValidateConfig`.
- [ ] **Step 3: `go test ./internal/features/`** — pass.
- [ ] **Step 4: Commit** `"feat(features): phase, per-surface visibility, audience, geo in flag config"`.

---

### Task 4: Caller-scoped resolution — `EvalFlag`, `AudienceResolver`, `GetAllEvaluated`

**Files:**
- Create: `internal/features/eval.go`
- Modify: `internal/features/features.go` (wire resolver)
- Test: `internal/features/eval_test.go`

**Interfaces:**
- Produces:
```go
// CallerScope — per-request attributes refining flag resolution.
type CallerScope struct {
	Surface     string            // validated X-JC-Surface, "" = none
	CountryCode string            // geoip GeoContext.CountryCode, "" = unknown
	GeoValid    bool              // GeoContext.IsValid
	GroupSlugs  map[string]bool   // caller's group slugs; nil = anonymous
	Permissions map[string]bool   // caller's resolved permission keys; nil = anonymous
	SuperAdmin  bool
}

type EvalResult struct {
	Enabled bool // routes may serve (Gate contract)
	Visible bool // appears in the surface's flag map
}

func SurfaceFrom(r *http.Request) string // header → "" if unknown
func (c *Checker) EvalFlag(ctx context.Context, key string, sc CallerScope) EvalResult
func (c *Checker) EvalAll(ctx context.Context, sc CallerScope) map[string]bool // Visible results for the flags map

// AudienceResolver supplies caller group/perm data without importing rbac.
// Verified signatures (internal/rbac/rbac.go:66,307,211):
//   UserContext{Groups []Group, Permissions map[string]bool}
//   (*RBAC).GetUserContext(ctx, userID) (*UserContext, error)
//   (*RBAC).IsSuperAdmin(ctx, userID) bool
type AudienceResolver interface {
	ResolveScope(ctx context.Context, userID pgtype.UUID) (groups map[string]bool, perms map[string]bool, superAdmin bool, err error)
}
func (c *Checker) SetAudienceResolver(r AudienceResolver)
func (c *Checker) BuildScope(r *http.Request, surface string) CallerScope // reads auth user + geo ctx + resolver
```

**Import direction verified:** `internal/features/handlers.go` already imports `internal/auth`; neither `internal/auth` nor `internal/geoip` imports `internal/features` — `eval.go` may import both directly. `features` must still not import `internal/rbac` (the `AudienceResolver` interface is the seam).

**Cache requirement (do not skip — perf):** `flagCacheEntry` currently stores only `enabled bool` (features.go:48). Scoped evaluation needs `config` too, or every gated request hits Postgres. Extend the entry to `{enabled bool, config []byte, createdAt}`; change the L2 Valkey value at `jc:flags:<key>` from a bare bool to JSON `{enabled, config}` — decode must accept the legacy bare `true`/`false` (config = `{}`) for in-flight stale entries. `EvalFlag`/`IsEnabled` share this cache; invalidate per-key on every write path (Task 6 hook ensures writes also clear it).

Also add an aggregate L2 entry `jc:flags:_all` (JSON array of `FlagRow`, TTL `flagCacheTTL`) backing `EvalAll` and `ListExtensions` — both need every row's config and must not run `ListFlagRows` on every request. Invalidate on any flag write. **Rule 4:** register `jc:flags:_all` in `.jeos/conventions/valkey-keys-catalog.md` (Task 15).

- [ ] **Step 1: Failing test table** for `EvalFlag` covering the spec §5.1 matrix:
  - disabled flag → `{false,false}` regardless.
  - enabled + no surface rules → `{true,true}`.
  - `hidden` on `main` + sc.Surface="main" → `{true,false}`; on "tools" → `{true,true}`.
  - `disabled` on `main` → `{false,false}`.
  - `geo_allow:[US]` + CountryCode="NG" → `{false,false}`; "US" → `{true,true}`; GeoValid=false → `{false,false}`.
  - `geo_deny:[NG]` + "NG" → `{false,false}`; GeoValid=false → `{true,true}`.
  - `audience:[group:beta]` + caller in beta → on; other group → off; anonymous (nil maps) → off; SuperAdmin=true → on.
  - `perm:beta.access` entry + perm map containing it → on.
  - `hidden`+audience: in-audience → `{true,false}`; out → `{false,false}`.
- [ ] **Step 2: Implement `eval.go`** — resolution order exactly as spec §5.1; `BuildScope` reads `auth.FromContext` (import verified clean — see note above), `geoip.FromContext`, and `audienceResolver.ResolveScope` when a user is present. Extend `flagCacheEntry` per the cache note above in the same pass.
- [ ] **Step 3: Tests pass; commit.**

---

### Task 5: Gate + flags endpoints become scope-aware

**Files:**
- Modify: `internal/features/features.go` (`Gate`), `internal/features/handlers.go` (`HandleGetFeatureFlags`, new `HandleGetFeatureFlagDetails`)
- Modify: `internal/server/server.go` (wire resolver adapter, register details route)
- Test: `internal/features/handlers_test.go` (add cases)

**Interfaces:**
- Consumes: `EvalFlag`, `EvalAll`, `SurfaceFrom`, `BuildScope`, `AudienceResolver` (Task 4).
- Produces: `GET /api/v1/feature-flags/details` → `{data: [{key, enabled, phase, rolloutPercentage, visible}]}` (public, same posture as the map endpoint). **Do NOT return `surfaces`/`audience`/`geo_*`** — group slugs, permission nodes, and geo policy are internal config (spec §5.3 updated); full rules are exposed only via `/api/v1/control/extensions`.

- [ ] **Step 1: Update `Gate`** — build scope via `c.BuildScope(r, SurfaceFrom(r))`, then `if !c.EvalFlag(ctx, key, sc).Enabled → 404 feature_disabled`. Signature unchanged (all ~80 call sites untouched). `BuildScope` must tolerate absent auth (anonymous → empty scope).
- [ ] **Step 2: Update `HandleGetFeatureFlags`** — return `EvalAll(ctx, scope).Visible` map instead of `GetAllEnabled`. Keep `{data: map[string]bool}` envelope. `EvalAll` reads the `jc:flags:_all` aggregate (Task 4), not `ListFlagRows` per request.
- [ ] **Step 3: `HandleGetFeatureFlagDetails`** — Registry+DB merge (reuse `ListFeatureFlags`' merge), each entry's `enabled`/`visible` evaluated for the caller scope; expose only `{key, enabled, phase, rolloutPercentage, visible}` — never `surfaces`/`audience`/`geo_*` (internal config).
- [ ] **Step 4: server.go wiring** — next to `featureHandlers := features.NewHandlers(featureChecker)` (~line 447):

```go
featureChecker.SetAudienceResolver(audienceResolverAdapter{rbac: rbacResolver})
```
plus the adapter near `literalResolverAdapter` (~line 7990):
```go
type audienceResolverAdapter struct{ rbac *rbac.RBAC }
func (a audienceResolverAdapter) ResolveScope(ctx context.Context, userID pgtype.UUID) (map[string]bool, map[string]bool, bool, error) {
	uc, err := a.rbac.GetUserContext(ctx, userID)
	if err != nil { return nil, nil, false, err }
	groups := map[string]bool{}
	for _, g := range uc.Groups { groups[g.Slug] = true }
	return groups, uc.Permissions, a.rbac.IsSuperAdmin(ctx, userID), nil
}
```
Register `r.Get("/feature-flags/details", featureHandlers.HandleGetFeatureFlagDetails)` beside line 3059.
- [ ] **Step 5: Tests + commit.**

---

### Task 6: Write-path config hook + audit

**Files:**
- Modify: `internal/features/features.go` (hook registry + fire on all Checker write paths), `internal/features/handlers.go`, `internal/server/server.go` (wire `SetOnConfigChanged` → roadmap sync — actual call lands in Task 8; wire it there).
- Test: `internal/features/handlers_test.go`

**Interfaces:**
- Produces: `func (c *Checker) SetOnConfigChanged(fn func(ctx context.Context, key string, cfg FlagConfig))` — **on `Checker`, not `Handlers`**: control extension writes go through `h.FeatureChecker.SetEnabled`/`SetRolloutPercentage`, never touching `featureHandlers`, so a Handlers-level hook would silently skip the `/control/extensions` write path. `Handlers.SetOnConfigChanged` becomes a thin delegate to the checker setter. Fired fire-and-forget (`context.WithoutCancel`, Rule 8) after every successful write.
- Consumed by: Task 8 (`roadmapService.SyncFlagPhase`).

- [ ] **Step 1:** Add `onConfigChanged` field + setter on `Checker`. Fire it inside `SetEnabled`, `SetRolloutPercentage`, `UpdateFlag`, and `DeleteFlag` (delete → fire with empty `FlagConfig{}` so roadmap unlinks per spec §4.3) — that covers admin CRUD, `/control/extensions` writes, and `HandleSetFeatureFlag` uniformly. Also `Del` the per-key cache entry and `jc:flags:_all` at the same points.
- [ ] **Step 2:** Extend `FeatureFlagInput` with `Phase`, `ShowOnRoadmap`, `Surfaces`; validate via `ValidateConfig` → `400` on error; write through `UpdateFlag` (already persists config).
- [ ] **Step 3:** `Handlers.SetOnConfigChanged` delegates to `checker.SetOnConfigChanged`; keep existing `SetOnFlagEnabled` behavior unchanged.
- [ ] **Step 4:** Audit — each write inserts `audit_logs` (action `feature_flag.update`, actor, `key`, before/after config). Follow the existing audit insert pattern used by other control handlers (grep `audit_logs` in `internal/control/handlers.go`).
- [ ] **Step 5: Tests + commit.**

---

### Task 7: Roadmap `testing` status

**Files:**
- Modify: `internal/roadmap/types.go`, `internal/roadmap/service.go`, `internal/roadmap/handlers.go` (any status validation paths)
- Test: `internal/roadmap/types_test.go`, `service_test.go`

**Interfaces:**
- Produces: `StatusTesting ItemStatus = "testing"`; in `PublicStatuses`, `VotableStatuses`, `AllStatuses`; `boardColumnOrder` gains `{StatusTesting, "Testing"}` between InProgress and Shipped; `validTransitions` gains `planned→testing`, `in_progress→testing`, `testing→shipped|in_progress|archived`, `under_review→testing` (community item can go straight to beta).

- [ ] **Step 1: Failing tests** — `ValidStatus("testing")` true; `IsVotableStatus(testing)` true; `IsPublicStatus(testing)` true; transitions in/out per above.
- [ ] **Step 2: Implement** in `types.go`. Verified: `ListPublicRoadmapItems` filters via `i.status = ANY(@statuses::text[])` — no SQL change needed; adding `StatusTesting` to `PublicStatuses` flows automatically. Still grep `internal/roadmap/` for any other inlined status lists (admin filters, votable checks).
- [ ] **Step 3: Tests + commit** `"feat(roadmap): testing status in lifecycle (spec §3.3)"`.

---

### Task 8: Roadmap `SyncFlagPhase` + linked-status enforcement

**Files:**
- Modify: `internal/db/queries/roadmap.sql` (+ `make sqlc-gen`)
- Modify: `internal/roadmap/store.go`, `service.go`, `handlers.go`, `types.go`
- Modify: `internal/server/server.go` (~line 885–888 + ~1324)
- Test: `internal/roadmap/service_test.go`

**Interfaces:**
- New sqlc queries:
```sql
-- name: GetRoadmapItemByFlagKey :one
SELECT * FROM roadmap_items WHERE feature_flag_key = $1 AND deleted_at IS NULL;
-- name: ListRoadmapItemsByFlagKeys :many
SELECT * FROM roadmap_items WHERE feature_flag_key = ANY($1::text[]) AND deleted_at IS NULL;
-- name: ClearRoadmapItemFlagKey :exec
UPDATE roadmap_items SET feature_flag_key = NULL, updated_at = now() WHERE feature_flag_key = $1;
```
(`ListRoadmapItemsByFlagKeys` is needed by Task 9 to batch `linked_roadmap_slug` — one query for the whole extensions list, not N.)
- New sentinel: `ErrStatusDerived = errors.New("status_derived")` in `internal/errors/sentinel.go` + a case in `internal/errors/http.go` → `409` code `status_derived` (mirror the `ErrInvalidStatusTransition` block at http.go:125).
- Produces: `func (s *Service) SyncFlagPhase(ctx context.Context, flagKey string, cfg features.FlagConfig) error` — implements spec §4.4. `internal/roadmap` → `internal/features` import is clean (features is a leaf; it imports only auth/geoip).

- [ ] **Step 1: Add queries + `make sqlc-gen`.**
- [ ] **Step 2: `SyncFlagPhase`** per spec: `showOnRoadmap=false`+linked → set status `archived`; `true`+no item → create (title=name, slug=`content.Slugify(name)`+unique suffix, summary=description, status=derived); `true`+item → write derived status directly via `UpdateRoadmapItemStatus` bypassing `IsValidTransition`; `phase→shipped` sets `shipped_at` (SQL-side CASE already handles this) + fires `ship_notify` only when previous status ≠ shipped; always `s.invalidateBoard(ctx)`. Empty `flagKey` (flag deleted) → `ClearRoadmapItemFlagKey`.
- [ ] **Step 3: Linked-status enforcement** — in `AdminSetStatus` (service.go:651), after `current` is loaded and `from` computed (~line 671): `if current.FeatureFlagKey.Valid && current.FeatureFlagKey.String != "" → return ErrStatusDerived`. In `AdminCreateItem`/`AdminUpdateItem`, validate `feature_flag_key` against `features.RegistryLookup` + existing flag rows (`SELECT 1 FROM feature_flags WHERE key=$1`); unknown → `400 unknown_feature_flag`. Setting a link applies the flag's current phase immediately. Add the `ErrStatusDerived` sentinel + http.go mapping in this step.
- [ ] **Step 4: Wire in server.go** near line 1324 — hook on the **checker** (Task 6):
```go
featureChecker.SetOnConfigChanged(func(ctx context.Context, key string, cfg features.FlagConfig) {
	if err := roadmapService.SyncFlagPhase(ctx, key, cfg); err != nil {
		slog.Error("roadmap sync flag phase failed", "key", key, "error", err)
	}
})
```
- [ ] **Step 5: Tests** — sync creates item on first enable, moves status on phase change, archives on show_off, clears link on delete, linked PATCH status → 409, bad flag key → 400. Commit.

---

### Task 9: Control backend — `ListExtensions` over Registry + extended PATCH

**Files:**
- Modify: `internal/control/handlers.go` (~lines 3333–3613)
- Test: `internal/control/handlers_test.go` (or new file per existing conventions)

**Interfaces:**
- Produces: `GET /control/extensions` item shape:
```json
{"key","name","description","enabled","version","setting_key","settings_category","category","rollout_percentage","phase","show_on_roadmap","surfaces":{"main":{"state":"visible","audience":[],"geo_allow":[],"geo_deny":[]},…},"linked_roadmap_slug"}
```
- `PATCH /control/extensions/{key}` body gains: `phase`, `show_on_roadmap`, `surfaces`, `name`, `description`, `rollout_percentage` — all optional; validated via `features.ValidateConfig`.

- [ ] **Step 1: Failing test** — list returns every Registry key (not just 28); PATCH with `{phase:"testing", surfaces:{main:{state:"visible",audience:["group:beta_testers"]}}}` persists; `phase:"bogus"` → 400.
- [ ] **Step 2: Implement** — `ListExtensions` iterates `features.Registry` ∪ `feature_flags` rows. **Batch, do not loop-query:** the page polls every 30s over ~60 flags — one `ListFlagRows` call (carries `Enabled`+`Config`; or the `jc:flags:_all` aggregate) + one `ListRoadmapItemsByFlagKeys` (Task 8) + merge in Go. Per-flag `IsEnabled`/`GetFlagConfig`/roadmap lookups are forbidden here (N+1). DELETE `knownExtensions`, `findExtension`, `extensionDefaultEnabled`, `extensionSettingKey` and their tests; unknown key → 404 `extension_not_found` but admin-created `feature_flags` rows outside the Registry still list (union, like `ListFeatureFlags`). `UpdateExtension` merges decoded body into existing `FlagConfig` (decode stored config, apply only present fields, `ValidateConfig`, `UpdateFlag`) — the `Checker` hook fires `onConfigChanged` automatically per Task 6.
- [ ] **Step 3:** `linked_roadmap_slug` — merge from the batched `ListRoadmapItemsByFlagKeys` map; unlinked → omit field.
- [ ] **Step 4:** `show_on_roadmap=true` while `roadmap.enabled` resolves off → still persist, but include `"warning": "roadmap_disabled"` in the response `data` (spec §6.3).
- [ ] **Step 5: Tests + commit.**

---

### Task 10: Control UI — `/extensions` card + Manage slide-over

**Files:**
- Modify: `apps/control/src/routes/extensions/+page.svelte`
- Create: `apps/control/src/lib/components/extensions/manage-sheet.svelte` (or colocate if the app has no components dir convention — check `apps/control/src/lib/components/`)
- i18n: `apps/control/src/lib/locales/en.json` + `fr.json` (`extensions.*` keys)

- [ ] **Step 1:** Extend the `Extension` interface with `phase`, `show_on_roadmap`, `surfaces`, `linked_roadmap_slug`; render phase chip + surface summary on each card.
- [ ] **Step 2:** Manage slide-over with the four spec sections (General / Rollout / Visibility / Roadmap). Visibility section: per surface — segmented control (Visible/Hidden/Disabled) + expandable audience multi-select (options fetched from control's group + permission-node list endpoints — reuse whatever the `permission-nodes` page calls) + geo multi-select (`GET /api/v1/geo/countries` via control api — verify route exists in api-routes-catalog first). Save calls the Task-9 PATCH with the full rule, e.g.:

```ts
await api(`/control/extensions/${ext.key}`, {
	method: 'PATCH',
	body: {
		phase: 'testing',
		show_on_roadmap: true,
		surfaces: { main: { state: 'visible', audience: ['group:beta_testers'], geo_allow: ['US'] } }
	}
});
```

Optimistic-update the card state and roll back on `result.ok === false`, matching the existing `setEnabled` pattern.
- [ ] **Step 3:** Preset buttons per spec ("Beta testers + staff", "Apply to all surfaces").
- [ ] **Step 4:** `bun run check` + `bunx svelte-check` clean; commit `"feat(control): unified extensions manager — phase, surfaces, audience, geo, roadmap link"`.

---

### Task 11: Admin roadmap form — flag select + derived status lock

**Files:**
- Modify: `apps/admin/src/routes/roadmap/+page.svelte` (field at ~line 259, detail ~527)
- Modify: `apps/admin/src/lib/api.ts` if a flag-list helper is missing

- [ ] **Step 1:** Replace `feature_flag_key` text input with `<select>` fed by `GET /admin/feature-flags` (keys + labels); include "" (no link).
- [ ] **Step 2:** When `row.feature_flag_key` set, status field read-only with hint `roadmap.status_derived_hint`; on `409 status_derived` show toast per existing error handling.
- [ ] **Step 3:** `bun run check`; commit.

---

### Task 12: `X-JC-Surface` + XFF forwarding in every app's API client

**Files:**
- Modify (browser clients): `apps/main/src/lib/api/client.ts`, `apps/tools/src/lib/api/client.ts`, `apps/academy/src/lib/api/client.ts`, `apps/docs/src/lib/api/client.ts`, `apps/admin/src/lib/api.ts`, `apps/control/src/lib/api.ts` (control sends `control` — harmless, never gated), `apps/extension` background fetch wrapper, `apps/studio-desktop`, `apps/mobile` (`japaconnect_core` API client)
- Modify (server clients — also verify XFF): `apps/main/src/lib/server/api.ts` (already forwards `clientIP` per ADR-064 — confirm), `apps/tools/src/lib/server/api.ts`, `apps/academy/src/lib/server/api.ts`, `apps/docs/src/lib/server/api.ts`

- [ ] **Step 1:** In each browser client, add `'X-JC-Surface': '<surface>'` to the shared header builder (`withCsrfHeader`-style merge point).
- [ ] **Step 2:** In each server client, add `X-JC-Surface` **and** verify `X-Forwarded-For: ctx.clientIP` is sent on every backend call (main does; port the pattern to tools/academy/docs if missing).
- [ ] **Step 3:** Ensure app-server egress IPs are in `clientip`'s trusted-proxy config (`JC_TRUSTED_PROXIES` or equivalent — check `internal/clientip/clientip.go:60`).
- [ ] **Step 4:** `bun run check` per app; commit per app or one commit `"feat: X-JC-Surface + visitor-IP forwarding for scoped flag resolution"`.

---

### Task 13: Migrate services off `system_settings` `*.enabled` → `FeatureChecker`

**Files:**
- Modify (one per service, ~15): `internal/story/service.go:170`, `internal/travel/service.go:688`, `internal/fx/service.go:81`, `internal/jobs/service.go:236`, `internal/visaroutes/service.go:167`, `internal/checklist/service.go:173`, `internal/wiki/service.go:441`, `internal/assessment/handlers.go:100`, `internal/remittance/service.go:346`, `internal/costofliving/service.go:161,299`, `internal/devices/service.go:190`, `internal/scholarships/service.go:463`, `internal/gamification/helpers.go:23-41`, plus any others found by:
  `grep -rn 'GetBoolOr.*\.enabled' internal/ --include='*.go' | grep -v _test`
- Modify: `internal/control/settings_catalog.go` — mark migrated `*.enabled` keys deprecated (keep catalog entry, prefix description `"DEPRECATED — use feature_flags/"` and exclude from the editable set the settings page renders).
- Modify: `.jeos/conventions/system-settings-catalog.md` — same deprecation notes.

- [ ] **Step 1:** Each service gains a `features.FeatureChecker` dep (or reuses an injected one); `s.settings.GetBoolOr(ctx,"x.enabled",d)` → `s.features.IsEnabled(ctx,"x.enabled")`. Update constructors + server.go wiring. Test fakes updated (`fakeSettingsReader`-style stubs gain the checker interface).
  - **Only reachability toggles migrate.** Sub-settings stay in `system_settings` per spec §9.2: `roadmap.submissions_enabled` (the §113 submission switch — reachability is already gated by `roadmap.enabled`), `gamification.badges_enabled`/`challenges_enabled`/`leaderboard_enabled`, `ai.cache_enabled`, `assessment.ai_roadmap_enabled`, `intel.relevance.classifier_enabled`, `wiki.candidate_reactions_enabled`-style feature-internal switches, `service_packages.tiers_enabled`, etc. Rule of thumb: if the key decides *whether the whole feature is reachable*, it becomes a flag; if it tunes a sub-behavior of an already-gated feature, it stays a setting. When in doubt, check whether the key appears in `features.Gate(...)` call sites — only those are reachability toggles.
  - Each migrated key must exist in the Registry (Task 2) so it appears on `/extensions`. Keys with no Registry entry and no `Gate` call (e.g. `intel.discovery.enabled` if it's service-internal) may need a new Registry entry or stay settings — decide per key and record the choice in the commit message.
- [ ] **Step 2:** Catalog deprecations.
- [ ] **Step 3:** `go build ./... && go test ./internal/...` — green; commit.

---

### Task 14: Public roadmap — "Testing" column + enriched badge

**Files:**
- Modify: `apps/main/src/routes/(app)/roadmap/+page.svelte` (+board component), `.../roadmap/[slug]/+page.svelte` (~line 105 badge), `apps/main/src/lib/types/index.ts`, `en.json`/`fr.json`

- [ ] **Step 1:** Board renders a "Testing" column between In Progress and Recently Shipped (column defs come from `board.columns` — the backend already ships label/order; verify frontend doesn't hardcode columns — if it does, map by status key).
- [ ] **Step 2:** Detail badge: `feature_flag_key` present → fetch flag phase via `/feature-flags/details` (SSR in `+page.server.ts`) → show `Rolling out — {p}%` when `0<p<100`, else phase label; `enabled=false` → "Paused".
- [ ] **Step 3:** i18n keys `roadmap.col_testing`, `roadmap.phase_*`; `bun run check`; commit.

---

### Task 15: JEOS + catalog updates

**Files:**
- Modify: `.jeos/sections/42-feature-flags.md` (or its successor — flag removal was noted "in progress"; amend to describe the unified model), `.jeos/sections/113-public-product-roadmap.md` (testing status + derived lifecycle), `.jeos/conventions/api-routes-catalog.md` (`/feature-flags/details`, extended control PATCH), `.jeos/conventions/system-settings-catalog.md` (deprecations), `.jeos/conventions/feature-flag-naming.md` (audience/geo note), `.jeos/conventions/valkey-keys-catalog.md` (new `jc:flags:_all` aggregate key + the `jc:flags:<key>` JSON-value format change), `.jeos/AGENTS.md` if a new golden rule is warranted (e.g. "feature toggles live only in feature_flags").

- [ ] **Step 1:** Amend each doc to match shipped behavior — JEOS is updated to reflect code, not aspirational text (Rule 1).
- [ ] **Step 2:** Commit `"docs(jeos): unified rollout model — phases, surfaces, audience, geo"`.

---

## Execution Handoff

Plan saved. Two execution options:

**1. Subagent-Driven (recommended)** — fresh subagent per task, review between tasks.

**2. Inline Execution** — I execute tasks in this session with checkpoints.

Which approach?
