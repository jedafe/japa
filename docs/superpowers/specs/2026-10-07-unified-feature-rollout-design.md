# Unified Feature & Extension Rollout Management — Design

**Date:** 2026-10-07
**Status:** Approved approach (flag-centric)
**Refs:** §42 (feature flags), §113 + ADR-127 (public roadmap), ADR-042 (404-not-403 gating), BL-013 (deterministic rollout hash), `internal/features`, `internal/control` (extensions), `internal/roadmap`

## 1. Problem

Feature/extension on/off state is managed from three places today:

- `feature_flags` table + `internal/features` Registry — the intended source of truth, read by `features.Gate` and `GET /api/v1/feature-flags`.
- `system_settings` `*.enabled` keys — still read directly by ~15 services (`travel`, `story`, `wiki`, `checklist`, `assessment`, `remittance`, `jobs`, `fx`, `push`, `visa_routes`, `gamification`, …) via `GetBoolOr`. The control panel settings page can still write them.
- `knownExtensions` — a hardcoded ~28-entry allowlist in `internal/control/handlers.go` that decides what `/control/extensions` can display. Many gated flags (`roadmap.enabled`, `mentorship.*`, `story/wiki/scam_alert/…`, `feature.dm`, `feature.jce.*`, `crs_calculator.enabled`, …) have no card, so they are invisible to operators.

Additionally, the public roadmap (§113) tracks shipping state manually per item even though `roadmap_items.feature_flag_key` already exists to link items to flags — the link is display-only.

**Goal:** one registry, one management surface (`/control/extensions`), one store (`feature_flags`). Every feature/extension is toggleable, carries a rollout phase that drives its roadmap status, and has per-surface visibility. The public roadmap becomes a projection of rollout state for opted-in features.

**Non-goals:**

- Per-user/per-cohort rollout. `rollout_percentage` remains the existing deterministic per-flag hash (BL-013) — a global all-or-nothing decision, not a user cohort split. Documented as a known limitation.
- Changing the roadmap's community submission flow (pending → triage stays manual).
- Gating the control panel itself — it is the management surface and always sees everything.
- No new RBAC nodes: control routes stay super-admin/`control_panel.access`-gated; roadmap admin keeps `roadmap.*` nodes.

## 2. Terminology

| Term | Meaning |
|---|---|
| **Flag** | A row in `feature_flags` (or its Registry default). Runtime gate state. |
| **Extension/Feature** | The catalog entry — Registry metadata (label, description, category, default surfaces) overlaid with the flag row. "Extension" and "feature" are the same thing in this design. |
| **Surface** | A client app: `main`, `tools`, `academy`, `docs`, `mobile`, `admin`, `extension`, `desktop`. Control panel excluded — always managed, never gated. |
| **Phase** | Rollout lifecycle on the flag: `planned → in_progress → testing → shipped`, plus `archived`. |
| **Surface state** | Per-surface visibility: `visible` (default), `hidden` (functional, not advertised), `disabled` (surface-scoped 404). |
| **Audience** | Optional per-surface restriction on *who* the flag is enabled for: user-group slugs (`group:beta_testers`) and/or permission nodes (`perm:beta.access`). Empty = everyone. |
| **Geo rule** | Optional per-surface restriction on *where* the flag is enabled: `geo_allow` ISO country codes (empty = all) and `geo_deny` codes (always wins). Resolved from the visitor's IP via `internal/geoip`. |
| **Linked item** | A `roadmap_items` row whose `feature_flag_key` points at a flag; its status is derived, not manual. |

## 3. Data model

### 3.1 `feature_flags.config` — extended jsonb

No schema change to the table. `FlagConfig` gains three fields (existing fields unchanged):

```go
type FlagConfig struct {
	Name              string                     `json:"name"`
	Description       string                     `json:"description"`
	Environment       string                     `json:"environment"`
	RolloutPercentage int                        `json:"rolloutPercentage"`
	Phase             string                     `json:"phase"`          // planned|in_progress|testing|shipped|archived
	ShowOnRoadmap     bool                       `json:"showOnRoadmap"`
	Surfaces          map[string]SurfaceRule     `json:"surfaces"`       // surface -> rule
}

// SurfaceRule is the per-surface visibility, audience, and geo policy for a flag.
type SurfaceRule struct {
	State    string   `json:"state"`               // visible|hidden|disabled
	Audience []string `json:"audience,omitempty"`  // group:/perm: entries; empty = everyone
	GeoAllow []string `json:"geo_allow,omitempty"` // ISO 3166-1 alpha-2; empty = all countries
	GeoDeny  []string `json:"geo_deny,omitempty"`  // ISO alpha-2 denied even if in GeoAllow
}
```

- `Phase` empty/absent is treated as `shipped` for enabled flags and `planned` for disabled ones (back-compat with all existing rows).
- `Surfaces` absent or missing a surface = `{state: "visible"}` (everyone, all countries). Unknown surface keys → 400 `invalid_surface`; unknown state values → 400 `invalid_surface_state`. Audience entries must be `group:`/`perm:` prefixed; anything else → 400 `invalid_audience`. Geo entries must be 2-letter ISO alpha-2 codes, stored uppercase; anything else → 400 `invalid_geo`.
- `ShowOnRoadmap` drives auto-create/sync/archive of the linked roadmap item (§5).

### 3.2 Registry becomes the single catalog

`internal/features.Feature` is extended to absorb everything `knownExtensions` carries:

```go
type Feature struct {
	Key              string            // "roadmap.enabled"
	Label            string
	Description      string
	Default          bool
	Category         string            // Content | Social | Commerce | Tools | AI | Engagement | Integrations | Platform
	SettingsCategory string            // control-panel settings category for the Configure link ("" = none)
	DefaultSurfaces  map[string]string // per-surface default state; absent = visible everywhere
}
```

`internal/control.knownExtensions` is deleted; `ListExtensions` iterates `features.Registry` and overlays DB rows (exactly the merge `ListFeatureFlags` already does). **All gated-but-unregistered keys must be added to the Registry** — currently missing: `roadmap.enabled` card metadata, `mentorship.enabled`, `mentorship.comparison`, `feature.dm`, `feature.follow`, `feature.forum`, `feature.reactions`, `feature.bookmarks`, `feature.ratings_reviews`, `feature.comparison`, `feature.academy`, `feature.tickets`, `feature.notifications`, `feature.wallet`, `feature.withdraw`, `feature.ai_agent`, `feature.jce.*` (incl. `feature.jce.data_sources`, `feature.jce.refresh`), `story/wiki/scam_alert/planning_group/digest/mutual_aid/calendar/japa_stage/forum.ama/achievement/service_*` flags, `crs_calculator.enabled`, `pof_calculator.enabled`, `eca_guide.enabled`, `sop_generator.enabled`.

Migration `000806_registry_backfill` seeds `feature_flags` rows for flags that are gate-enforced but unseeded, preserving current production state (copy `system_settings` `*.enabled` bools → `feature_flags.enabled` where no flag row exists — see §7).

### 3.3 `roadmap_items.status` — add `testing`

Migration `000806` also drops and recreates the `status` CHECK to include `'testing'` between `in_progress` and `shipped`. `roadmap.PublicStatuses` and `VotableStatuses` gain `testing` (beta items accept votes — gathering feedback is the point of a public beta). `validTransitions` gains `planned → testing` and `testing → shipped|in_progress|archived`. The board renders a "Testing" column between "In Progress" and "Recently Shipped".

## 4. Phase model and status derivation

### 4.1 Flag phases

```
planned → in_progress → testing → shipped
   ↘________archived (from any phase)
```

Phase transitions are free (no state machine enforcement beyond validating the value) — phases are operational metadata, not safety-critical. Every phase change writes `audit_logs` (actor, key, from, to).

**Defaults:** existing enabled flags = `shipped`; new/disabled flags = `planned`; admin-created keys = `planned` unless specified.

### 4.2 Derivation rule

When `roadmap_items.feature_flag_key` is set, the item's `status` is **owned by the flag**:

| Flag phase | Item status |
|---|---|
| `planned` | `planned` |
| `in_progress` | `in_progress` |
| `testing` | `testing` |
| `shipped` | `shipped` (sets `shipped_at`, fires `roadmap:ship_notify`) |
| `archived` | `archived` |

`PATCH /admin/roadmap/items/{id}/status` on a linked item returns `409 status_derived` — the operator must change the flag phase instead. Unlinking (PATCH with `feature_flag_key: ""`) leaves the item at its last derived status and returns it to manual control.

### 4.3 Show-on-roadmap lifecycle

When `show_on_roadmap` flips **on** (via control PATCH or admin flag update):

1. If a linked `roadmap_items` row already exists (re-enable) → unarchive it, set status from phase.
2. Otherwise the roadmap service creates a row: `title` = flag name, `slug` = slugified key (+ `-shortid` on conflict), `summary` = flag description, `status` = derived, `feature_flag_key` = flag key, `submitted_by` = NULL (admin-created).

When it flips **off**: the linked item is set `archived` (votes and comments preserved; board hides it).

When a flag row is **deleted** (`DELETE /admin/feature-flags/{key}`): linked items keep their current status but `feature_flag_key` is cleared, returning them to manual lifecycle.

Community submissions are untouched: `pending → under_review` stays manual. Approving a submission may optionally link it to a flag (new optional `feature_flag_key` field in the triage PATCH); once linked, it follows §4.2.

### 4.4 Sync mechanism

`features.Handlers` gains a second hook beside `SetOnFlagEnabled`:

```go
// fires after any write that changed a flag's config (phase/surfaces/
// show_on_roadmap) — used by roadmap.Service to sync linked items.
SetOnConfigChanged(func(ctx context.Context, key string, cfg FlagConfig))
```

Wired in `server.go` next to the existing `SetOnFlagEnabled` (line ~1324): the callback calls `roadmapService.SyncFlagPhase(ctx, key, cfg)` — fire-and-forget via `context.WithoutCancel`, same as the wiki seed hook (Rule 8). All flag write paths (`HandleSetFeatureFlag`, `CreateFeatureFlag`, `UpdateFeatureFlag`, `UpdateExtension`, `UpdateExtensionRollout`, control config PATCH) fire it.

`roadmap.Service.SyncFlagPhase`:

- `show_on_roadmap=false` + linked item → archive item, clear nothing (link retained for re-enable).
- `show_on_roadmap=true`, no item → create per §4.3.
- `show_on_roadmap=true`, item exists, phase changed → write the derived status directly, bypassing `IsValidTransition` (the flag's phase graph is the authority; the manual state machine applies only to unlinked items). Invalidate `jc:roadmap:board`; fire `roadmap:ship_notify` on `→shipped`.

## 5. Visibility model (per-surface states + audience)

### 5.1 State resolution

```
global enabled=false                       → 404 everywhere (existing Gate behavior)
surface state = disabled                   → 404 for that surface only
geo_deny contains caller's country         → flag OFF for that caller on that surface
geo_allow non-empty, country not in it     → flag OFF for that caller on that surface
audience set + caller not in audience      → flag OFF for that caller on that surface
surface state = hidden                     → routes work; excluded from surface's flag map
surface state = visible (default)          → normal
(OFF = 404 via Gate + absent from the caller's flags map)
```

**Audience** is orthogonal to state — it refines *who* a `visible`/`hidden` state applies to:

- `visible` + `audience: [group:beta_testers]` → shown in nav and functional **only** for beta testers; everyone else gets it disabled on that surface. This is the canonical "testing" configuration.
- `hidden` + audience → nav-hidden for everyone; functional only for members (non-members → 404).
- `disabled` → off for all on that surface; audience/geo irrelevant.
- Caller matches audience when **any** entry matches: `group:<slug>` = user's group slug, `perm:<node>` = user holds that permission node (checked via the RBAC resolver — additive with, not a replacement for, route-level `rbac.RequirePermission`).
- Anonymous callers are never in an audience (empty group/no perms) → audience-scoped flags are off for them.
- `super_admin` bypasses audience checks — operators managing a rollout must never be locked out of their own feature.
- Performance: the global enabled/rollout result stays on the existing L1/L2 cache; audience and geo checks are per-request overlays evaluated against the caller's already-resolved groups/permissions and the request-context `GeoContext` — no extra DB hit on the hot path.

**Geo** is orthogonal too — it refines *where* a state applies:

- `visible` + `geo_allow: [US, CA]` → feature on only for visitors/users whose IP geo-resolves to US or Canada. Everyone else: flag off on that surface.
- `visible` + `geo_deny: [NG]` → on everywhere except Nigeria.
- Both set → deny wins (`geo_allow: [US], geo_deny: []` vs `geo_allow: [], geo_deny: [NG]` compose naturally; a code in both lists is denied).
- Resolution reads `geoip.FromContext(r.Context())` — populated globally by `geoip.Middleware` (wired at server.go:241) which resolves `clientip.FromRequest(r)`. No header trust involved: unlike `X-JC-Surface`, geo is enforced server-side and can't be forged by the client.
- **Unknown country fails closed for allow-lists, open for deny-lists:** `IsValid=false` (private IP, missing GeoLite DB, lookup miss) never satisfies `geo_allow`, and never matches `geo_deny`. A feature gated to "US only" goes dark when geo is unavailable; a feature denying "NG" stays on.
- **SSR caveat — required, not optional:** SSR apps (main, academy, docs, tools) call the API server-side, so `RemoteAddr` is the app server, not the visitor. Each app's server-side API client must forward the visitor IP via `X-Forwarded-For` (clientip only honors XFF from trusted proxies — app server IPs/origins go in the trusted-proxy config), alongside `X-JC-Surface`. Direct browser→API calls (admin/control SPAs, mobile, extension) need no forwarding.
- Geo is **IP-based** (the Facebook model), not profile-country — it works identically for anonymous visitors and logged-in users, and follows the person, not the account. VPNs are a known evasion vector; geo is a rollout/compliance lever, not a security boundary.
- `super_admin` does NOT bypass geo rules — geo is location truth, not authorization; an admin in NG testing a US-only feature should see what a US user sees is false. (Use a staging env or VPN to preview geo-gated features.)

### 5.2 Surface detection — `X-JC-Surface`

Each app's API client sends a header identifying its surface:

| Header value | App |
|---|---|
| `main` | apps/main (SSR forwards it server-side in `+page.server.ts`/`+layout.server.ts` fetches) |
| `tools` | apps/tools |
| `academy` | apps/academy |
| `docs` | apps/docs |
| `mobile` | apps/mobile (japaconnect_core API client) |
| `admin` | apps/admin |
| `extension` | apps/extension (WXT background) |
| `desktop` | apps/studio-desktop |

`features.Gate` and the flags endpoints resolve the surface via a shared helper: `features.SurfaceFrom(r)` → header value if in the known set, else `""`. Unknown/absent → treated as no surface: global `enabled` applies, and the flag is *included* in the flag map (back-compat for public API consumers, which don't render nav). **The header is advisory and spoofable by design** — `hidden` is a discovery control, not an authorization boundary. Anything requiring hard isolation uses `disabled` or global `enabled=false`; audience checks are the exception — they ARE authorization and are enforced server-side against the caller's resolved groups/permissions, never the header.

### 5.3 `GET /api/v1/feature-flags` — surface-, audience-, and geo-scoped

Response stays `{data: {key: bool}}` (back-compat). The endpoint gains `OptionalAuth`: with a valid `X-JC-Surface`, `bool` = `enabled && surface != hidden && surface != disabled && caller ∈ audience (when set) && caller's country passes geo rules` and rollout-hash passes. Anonymous callers see only flags with no audience restriction. Apps consult this map for nav exactly as they do today — hidden/disabled/out-of-audience features simply read false. Because the result is caller-dependent, the per-user map is NOT written to the shared L1/L2 flag cache (only the underlying per-flag global resolution is cached).

New richer endpoint for surfaces that need phase data (e.g. "Coming soon"/"Beta" badges on cards):

```
GET /api/v1/feature-flags/details   → {data: [{key, enabled, phase, rolloutPercentage, surfaces:{…}}]}
```

Same public/no-auth posture as the existing map endpoint (it reveals only what an attacker could already probe via route 404s).

### 5.4 Enforcement points

- **Route gating:** `features.Gate` — unchanged signature; internally checks global enabled then surface-disabled.
- **Nav/UI:** every app's existing flag-map consumption (main `routeFeatureFlags.ts`/`LeftNav`, tools `registry.ts`, academy/docs layouts, mobile router) — unchanged code paths; the map just contains less for hidden surfaces.
- **Admin sidebar:** admin nav consults the same map with `admin` surface — hidden features' admin pages are unreachable through nav but deep links still load (they're hidden-but-functional); `disabled` on admin hides admin CRUD too.
- **Admin panel (`/api/v1/admin/feature-flags`)** stays functional — `FeatureFlagInput` gains `phase`, `showOnRoadmap`, `surfaces` passthrough so both write paths land in the same blob. The canonical UI remains control `/extensions`.

## 6. Control panel — `/extensions` becomes the center stage

Route stays `/extensions` (existing deep links + i18n preserved); title becomes "Extensions & Features".

### 6.1 Card changes

Each card (now rendered for every Registry entry) shows: name, key, category, phase chip (`planned`/`in_progress`/`testing`/`shipped`/`archived`), enabled switch, rollout %, roadmap badge when `show_on_roadmap`, and a surface-state summary (`main ✓ · tools hidden · admin ✓ …`).

### 6.2 Manage slide-over

Cards gain a "Manage" action opening a slide-over (mirroring the admin panel's `SlideOver` pattern, adapted to control's component set) with four sections:

- **General** — name, description, category (writes config blob); enabled toggle.
- **Rollout** — phase select + `rollout_percentage` input (existing PATCH …/rollout endpoint).
- **Visibility** — per-surface 3-state segmented control (Visible / Hidden / Disabled), defaults shown from `DefaultSurfaces`; each surface row optionally expands to an audience picker (multi-select of user groups + permission nodes, fed from `/control` group and permission-node lists) and a geo picker (country multi-select fed from the existing `GET /geo/countries` handler, in `geo_allow`/`geo_deny` mode). Preset shortcuts: "Beta testers + staff" applies `audience: [group:beta_testers]` (the standard `testing` config); "Apply to all surfaces" writes one rule to every surface.
- **Roadmap** — `show_on_roadmap` toggle, linked item slug/status/link to admin roadmap page, warning when `roadmap.enabled` is off.

### 6.3 API surface

```
GET   /control/extensions                     — union of Registry + DB rows (adds phase, surfaces, show_on_roadmap, linked_item_slug)
PATCH /control/extensions/{key}               — extended body: {enabled?, phase?, surfaces?, show_on_roadmap?, name?, description?, rollout_percentage?}
POST  /control/extensions/{key}/enable|disable — unchanged
PATCH /control/extensions/{key}/rollout        — unchanged (back-compat)
```

All writes: update `feature_flags` via `Checker`, `ClearCache()`, fire `onConfigChanged`, write `audit_logs`. Invalid phase/surface values → `400 invalid_phase` / `400 invalid_surface_state`. `show_on_roadmap=true` while `roadmap.enabled` is off → allowed, but response carries `"warning": "roadmap_disabled"`.

### 6.4 Roadmap admin form

`apps/admin/roadmap` item form: `feature_flag_key` free-text replaced by a `<select>` populated from `GET /admin/feature-flags` keys; a "clear link" option; linked items show status as read-only with a hint ("status follows the flag's rollout phase").

Public board: linked items render the existing flag badge, now enriched — "Testing" column added; detail page badge shows phase label (`Rolling out — 40%` when `0 < rolloutPercentage < 100`, else phase name).

## 7. Consolidating `system_settings` `*.enabled`

To make `feature_flags` genuinely sole-sourced:

1. `000806` migration: for every `*.enabled` bool key in the settings catalog that corresponds to a Registry flag, `INSERT … SELECT` copy `system_settings.value → feature_flags.enabled` **where no flag row exists** (never overwrite an explicit flag decision).
2. Services switch `settings.GetBoolOr("X.enabled", …)` → `FeatureChecker.IsEnabled("X.enabled")`. The services already take `settingsService`; they gain a `FeatureChecker` dependency (the interface `features.FeatureChecker` exists for exactly this).
3. The `*.enabled` bool entries are removed from the settings catalog's editable set (marked deprecated in `system-settings-catalog.md`; rows left in place harmlessly). Non-enabled keys (`*.subdomain`, quotas, etc.) stay in settings — that's their rightful home.
4. Settings page hides deprecated keys; a code comment links the catalog file to this spec.

## 8. Edge cases

- **Disabling a flag whose item is on the board:** the roadmap item stays (status unchanged); its routes 404. The badge renders "Paused" when `enabled=false` (item detail already tolerates flag state). Auto-archiving on disable was rejected — off/on is a kill switch, not a lifecycle move.
- **`roadmap.enabled` itself off:** roadmap board 404s; `show_on_roadmap` writes still allowed (warned); sync no-ops harmlessly.
- **Phase → shipped twice / item already shipped:** `shipped_at` preserved on re-ship; `ship_notify` deduped by checking previous status != shipped.
- **`hashInRollout` limitation:** rollout % is global-deterministic, not per-user. If true cohort rollout is ever needed, that's a separate ADR — out of scope.
- **Admin-created flag keys** (not in Registry): appear on /extensions with `Category: "Custom"`, `DefaultSurfaces` all-visible. They can carry phases/roadmap links like any flag.
- **Unknown `X-JC-Surface`:** ignored (treated as absent) — never used to *increase* exposure.
- **Audience group renamed:** config stores slugs, so a rename orphans the restriction (audience silently matches nobody → feature dark). The control UI warns on save when a referenced slug no longer exists; admin can then pick the renamed group. Acceptable because renames are admin-initiated and rare.
- **In-audience user without the route's permission node:** still 403 from `rbac.RequirePermission` — audience widens nav visibility, never bypasses route RBAC.
- **GeoLite DB missing/unloaded:** `geoip.NewReader` degrades to all-invalid `GeoContext`; per the fail-closed rule, allow-listed features go dark while deny-listed stay on. Control panel surfaces a warning badge on any flag carrying geo rules when `geoip.Reader.IsLoaded()` is false.
- **SSR without forwarded IP:** same as unknown country — the app server's IP usually geo-resolves to the host's datacenter country, which is *valid* but wrong (e.g. a German VPS makes a US-only feature dark for all SSR visitors). Mitigation = the required XFF forwarding in §5.1; Playwright smoke test asserts a forwarded US IP unlocks a US-only flag.
- **Geo-gated feature on the roadmap:** the board is global — a linked item stays visible regardless of geo (the roadmap communicates intent, not access); the flag's routes still geo-gate normally.

## 9. Testing

- **Go:** table tests for `SurfaceFrom`, surface-state resolution matrix (enabled × hidden × disabled × no-header), audience evaluation (group match, perm match, anonymous → off, super_admin bypass, renamed/missing group → off), geo evaluation (allow match/miss, deny match/miss, both-set deny-wins, invalid GeoContext → allow-fails/deny-passes, super_admin does NOT bypass geo), `SyncFlagPhase` transitions, `status_derived` rejection on linked items, config validation errors, merge of Registry+DB in `ListExtensions`.
- **Migration:** up/down for the CHECK constraint; backfill idempotent (`ON CONFLICT DO NOTHING`).
- **Frontend:** `bun run check` on control/admin/main/tools; Playwright smoke — toggle a feature hidden on `main`, confirm nav drops it and direct URL still works; flip `show_on_roadmap`, confirm card appears in the right column; phase `→ testing` moves the card.
- **Regression:** feed/search/sitemap still exclude `content_type='roadmap'`; `/feature-flags` response shape unchanged for header-less callers.

## 10. Rollout plan (implementation order)

1. Migration `000806` (CHECK extension + settings→flags backfill) + Registry metadata expansion + missing flag registrations.
2. `FlagConfig` extension + write-path hooks + `onConfigChanged` wiring.
3. `roadmap` module: `testing` status, `SyncFlagPhase`, linked-status derivation, admin validation/select.
4. Control `/extensions`: catalog switch to Registry, extended PATCH, card + slide-over UI.
5. `X-JC-Surface` header + `X-Forwarded-For` visitor-IP forwarding in each app's API client (SSR apps also add their IPs to the trusted-proxy config) + `Gate`/flags-endpoint surface, audience, and geo resolution.
6. Service `GetBoolOr("*.enabled")` → `FeatureChecker` migration + settings catalog deprecation.
7. Board "Testing" column + enriched badges; JEOS section updates (§42 amendment, §113 amendment, settings catalog, api-routes-catalog).
