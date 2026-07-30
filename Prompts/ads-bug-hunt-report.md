# Bug Hunt Report — `/ads` (Admin Panel)

**Target:** `http://localhost:3001/ads` and `http://localhost:3001/ads/new`
**Date:** 2026-07-30
**Scope:** Frontend (Svelte 5, Bits UI, DataTable, i18n, RBAC), Backend (handlers, store, sqlc, SQL), JEOS spec compliance.

---

## Summary

| Severity | Count | Status |
|----------|-------|--------|
| Critical (page-breaking) | 3 | Fixed |
| High (contract drift / RBAC) | 5 | Fixed |
| Medium (i18n / token classes) | 2 | Fixed |
| Low (defensive guards) | 2 | Fixed |
| **Total** | **12** | **12 fixed** |

All fixes verified end-to-end with Playwright: list renders, empty states show, campaign create → redirect → row appears, activate/pause/end status transitions work, stats modal loads with 200 (was 500), slot-type dropdown shows real names (was blank `(x)`). `svelte-check` 0 errors, `go build ./...` clean.

---

## Bugs Found & Fixed

### BUG-1 — CRITICAL: Nil slice serialized as `null` breaks list page
**File:** `internal/advertising/handlers.go`
**Symptom:** `/ads` shows "Failed to load campaign" even though `GET /admin/ads/campaigns` returns 200 OK.
**Root cause:** `ListCampaigns` returns `[]AdCampaign` from sqlc. When the tenant has zero campaigns, Go serializes a nil slice as JSON `null` (not `[]`). The frontend did `data.items` on `null` → TypeError → catch block → error banner.
**Fix:** Added `nonNilSlice[T any]` helper; applied to `ListCampaigns`, `ListSlotTypes`, `ListBoosts`, `ListActiveBoosts` handlers so empty results emit `[]`.
**Ref:** §6.2 API response format — nil slices must serialize as `[]`.

### BUG-2 — CRITICAL: `GetCampaignStats` SQL queries non-existent columns (500)
**File:** `internal/db/queries/advertising.sql` (+ regenerated `internal/db/gen/advertising.sql.go`)
**Symptom:** Clicking the stats (eye) button → `GET .../stats` returns HTTP 500.
**Root cause:** The query did `SUM(impressions)`, `SUM(clicks)`, `SUM(conversions)` on `ad_events`, but `ad_events` has no such columns — it stores one row per event with an `event_type` discriminator (`'impression'` / `'click'`). The query failed at runtime with a column-doesn't-exist error.
**Fix:** Rewrote the query to aggregate by counting rows per `event_type` using `COUNT(*) FILTER (WHERE event_type = 'impression')` etc. Ran `make sqlc-gen` to regenerate. (Rule 2 — never hand-edit generated files.)
**Ref:** `migrations/000024_advertising.sql` table definition; `sections/25-ad-placement.md`.

### BUG-3 — CRITICAL: `getCampaignStats` store returned incomplete contract
**File:** `internal/advertising/store.go`
**Symptom:** Stats modal fields (spend, cpc, ctr, daily_stats, campaign_id) rendered as `undefined`/blank.
**Root cause:** Store map only included `impressions/clicks/unique_users/conversions/ctr`. Frontend `CampaignStats` type expects `campaign_id, spend, cpc, daily_stats` additionally.
**Fix:** Added all contract fields with zero-defaults; `cpc` derived from spend/clicks; `daily_stats` defaults to `[]` (rollup not yet wired). `spend` defaults to 0.0 until `ad_campaigns.spent_credits` aggregation lands.
**Ref:** `sections/25-ad-placement.md` stats contract.

### BUG-4 — HIGH: Frontend RBAC node name drift (`ads.manage` vs `ad.campaign.manage`)
**File:** `apps/admin/src/routes/(app)/ads/+page.svelte`
**Symptom:** Non-super-admin users with the real `ad.campaign.manage` permission couldn't see action buttons; the page checked `ads.manage` which is an unseeded typo that only super_admin bypasses via the `*` wildcard.
**Fix:** Changed `hasPermission('ads.manage')` → `hasPermission('ad.campaign.manage')` (the node seeded in migration `000109_rbac_permission_reconciliation.up.sql` and enforced by the backend `RequirePermission` middleware).
**Ref:** `sections/03-rbac.md`.

### BUG-5 — HIGH: Slot-type dropdown rendered blank `(x)` options (contract drift)
**File:** `apps/admin/src/routes/(app)/ads/new/+page.svelte`
**Symptom:** Slot Type `<select>` showed options like `(x)` with no name.
**Root cause:** Frontend `SlotType` type assumed `name`, `width`, `height` fields. The backend `AdSlotType` sqlc struct exposes `slot_name` and `max_display_per_page` (no width/height at all).
**Fix:** Updated `SlotType` type to `slot_name` / `max_display_per_page`; dropdown now renders `feed_inline (2/page)` etc.; `slotTypeName` binds to `slot.slot_name` which `CreateCampaign` resolves via `GetAdSlotTypeByName`.
**Ref:** `internal/db/gen/models.go` `AdSlotType` struct; `sections/25-ad-placement.md`.

### BUG-6 — HIGH: Local state mutation after mutation (Rule 16 violation)
**File:** `apps/admin/src/routes/(app)/ads/+page.svelte`
**Symptom:** `toggleCampaign`/`endCampaign` patched `campaign.status` directly and reassigned `campaigns = campaigns` — UI could diverge from server truth on partial/failed writes.
**Fix:** All mutation handlers now `await load()` to re-fetch from the server after a successful write, and surface `result.error` on failure. Matches Rule 16 (re-fetch, don't patch).
**Ref:** AGENTS.md Rule 16.

### BUG-7 — MEDIUM: Missing i18n keys (tab labels, column headers, form labels)
**Files:** `apps/admin/src/lib/locales/en.json`, `apps/admin/src/lib/locales/fr.json`
**Symptom:** Tab labels and column headers used hardcoded English strings; new-page form labels had no i18n keys.
**Fix:** Added 33 new `ads.*` keys to both en.json and fr.json (tab_campaigns, tab_boosts, col_*, deleting, delete_selected, end, end_confirm, end_boost_confirm, new, new_desc, basic_info, campaign_name, slot_type, select_slot, target_url, content, headline, body, cta, budget_schedule, priority, frequency_cap, start_date, end_date, geo_targeting, geo_helper, target_countries, target_spaces, search_spaces, name_required, created, create_error, no_permission). Page now uses `t('ads.col_*', 'fallback')`.
**Ref:** i18n convention — all visible strings translatable.

### BUG-8 — MEDIUM: statusBadge used non-token CSS classes
**File:** `apps/admin/src/routes/(app)/ads/+page.svelte`
**Symptom:** `draft` status used `bg-info-100 text-info-800 border-info-200` — non-token shades that don't exist in the design system and break dark mode. `completed` used bare `border`.
**Fix:** Replaced with semantic token classes: `bg-info/15 text-info border-info`, `border-border`, and added `ended`/`pending_review`/`rejected` statuses.
**Ref:** design-system token convention.

### BUG-9 — LOW: No null-guard on `result.data` in list loader
**File:** `apps/admin/src/routes/(app)/ads/+page.svelte`
**Symptom:** Even after BUG-1, a defensive guard was missing on the frontend side.
**Fix:** `const data = (result.data ?? []) as ...` so a null `data` never throws on `.items` access. Also fixed boost search filter (was filtering on `b.username` which doesn't exist on boosts → now `b.boost_type`).

### BUG-10 — LOW: No null-guard on new-page form data loaders
**File:** `apps/admin/src/routes/(app)/ads/new/+page.svelte`
**Symptom:** `slotsResult.data` / `spacesResult.data` could be null.
**Fix:** Added `?? []` guards and surfaced `slotsResult.error` when not ok.

---

## Files Changed

| File | Change |
|------|--------|
| `internal/advertising/handlers.go` | `nonNilSlice` helper; applied to 4 list handlers; rewrote `UpdateCampaign` + `GetSlotType` to use service/generated queries |
| `internal/advertising/service.go` | `getCampaignStats` full contract; added `UpdateCampaignDetails` + `GetSlotType` methods |
| `internal/advertising/store.go` | `getCampaignStats` full contract + spend/cpc defaults |
| `internal/db/queries/advertising.sql` | `GetCampaignStats` query rewritten (COUNT FILTER); added `UpdateAdCampaignDetails` query |
| `internal/db/gen/advertising.sql.go` | Regenerated via `make sqlc-gen` |
| `apps/admin/src/routes/(app)/ads/+page.svelte` | RBAC node, null-guards, Rule 16 reloads, i18n, statusBadge tokens, Campaign status union |
| `apps/admin/src/routes/(app)/ads/new/+page.svelte` | SlotType contract fix, null-guards, error surfacing |
| `apps/admin/src/lib/locales/en.json` | +33 `ads.*` keys |
| `apps/admin/src/lib/locales/fr.json` | +33 `ads.*` keys (FR translations) |

---

## Verification

- `go build ./...` — clean
- `make sqlc-gen` — regenerated successfully
- `cd apps/admin && bun run check` (svelte-check) — 0 errors, 0 warnings
- Playwright end-to-end:
  - `/ads` list renders with empty state "No campaigns created yet" (no error banner)
  - Boosts tab shows "No boosts" empty state
  - `/ads/new` slot dropdown shows real slot names: `comparison_sponsored (2/page)`, `feed_inline (2/page)`, `forum_inline (2/page)`, `search_sponsored (3/page)`, `sidebar (1/page)`
  - Create campaign → redirect to `/ads` → row appears with correct name/status/budget
  - Activate (draft→active), Pause (active→paused), End (→ended) all return 200 and reload
  - Stats modal opens, `GET .../stats` returns 200 (was 500), shows Impressions/Clicks/Conversions/Spend/CTR/CPC
  - No console errors throughout

---

## Not Fixed (Out of Scope / Future)

- **`daily_stats` aggregation** — the stats response returns `[]` for `daily_stats` because no daily rollup table/query exists yet. Wiring `ad_analytics_daily` into the stats endpoint is a future enhancement, not a bug in the current page.
- **`spend` aggregation** — defaults to 0.0 until `ad_campaigns.spent_credits` (or a spend ledger) is summed into the stats query.

---

## Additional Bugs Fixed (Follow-up)

### BUG-11 — HIGH: `UpdateCampaign` raw SQL referenced non-existent `description` column (500)
**File:** `internal/advertising/handlers.go`, `internal/advertising/service.go`, `internal/db/queries/advertising.sql`
**Symptom:** `PATCH /admin/ads/campaigns/:id` would 500 — raw SQL did `UPDATE ad_campaigns SET ... description = COALESCE(...)` but `ad_campaigns` has no `description` column.
**Fix:** Added `UpdateAdCampaignDetails` sqlc query (partial PATCH: name + budget via COALESCE, no description). Added `Service.UpdateCampaignDetails` method. Rewrote handler to use the service method instead of raw SQL (Rule 2). Handler now returns the full updated campaign object (was just `{id}`).
**Verified:** `PATCH` with `{"name":"Updated Test Campaign","budget":250}` → 200, returns full campaign with updated fields.

### BUG-12 — HIGH: `GetSlotType` raw SQL referenced non-existent `width`/`height` columns (500)
**File:** `internal/advertising/handlers.go`, `internal/advertising/service.go`
**Symptom:** `GET /admin/ads/slot-types/:id` would 500 — raw SQL did `SELECT id, name, ..., width, height FROM ad_slot_types` but the table has `slot_name`/`max_display_per_page` (no `name`/`width`/`height`).
**Fix:** Added `Service.GetSlotType` method using the existing generated `GetAdSlotTypeByID` query. Rewrote handler to use the service method. Now returns the full `AdSlotType` struct with correct field names. 404 on missing ID via `handleServiceError` (pgx.ErrNoRows → `not_found`).
**Verified:** `GET` with real ID → 200 with `slot_name`/`max_display_per_page`/`is_active`; bad ID → 404 `{"error":{"code":"not_found",...}}`.
