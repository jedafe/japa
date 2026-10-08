# Ads System Completion — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all six gaps in the §25/§61 ads subsystem, add a house-ad fallback fill tier, ten new ad placements (in-article, native, sticky, comments), and expand the ad-network provider enum to 25 values.

**Architecture:** Extend the existing `internal/advertising` + `internal/adsselfserve` packages — no new packages. Serving stays two-pass (paid tier → house tier) via a new targeting-aware sqlc query. Analytics rollup is written at event-ingest time; spend accrual runs as a new daily cron task. Frontend gains three new components (`InArticleAd`, `NativeAd`, `StickyAnchorAd`) mounted into existing surfaces.

**Tech Stack:** Go 1.24+ (chi, pgx, sqlc, asynq), PostgreSQL, golang-migrate, SvelteKit 2 + Svelte 5 runes, bun.

## Global Constraints

- JEOS is updated **before** code (AGENTS.md Rule 1). Task 1 must land first.
- Never hand-edit `internal/db/gen/**` — edit `internal/db/queries/*.sql`, then `make sqlc-gen`.
- New Asynq task names MUST be added to `.jeos/sections/11-job-queue-asynq.md` before use in code (Rule 7).
- API errors use `errpkg.WriteError(w, status, "snake_case_code", "...")`.
- All commits go to `main`. No AI attribution in commits/comments (Rule 17).
- Frontend: bun only (`bun run check`, `bunx svelte-check`), Svelte 5 runes, no hardcoded data (Rule 16).
- sqlc reads `migrations/` as schema — the 000812 `.up.sql` must exist before `make sqlc-gen`.

---

### Task 1: JEOS spec amendments

**Files:**
- Modify: `.jeos/sections/25-ad-placement.md`
- Modify: `.jeos/sections/61-self-serve-ads.md`
- Modify: `.jeos/sections/11-job-queue-asynq.md`
- Modify: `.jeos/conventions/asynq-tasks.md`
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/conventions/system-settings-catalog.md`
- Modify: `.jeos/schemas/MIGRATION-ORDER.md`
- Modify: `.jeos/roadmaps/ROADMAP-BACKEND.md`

- [ ] **Step 1: Amend §25** — append to `Decided` section and add a new `## House Ads (fill tier)` + `## Serving Targeting` + `## Additional Slot Types` + `## Ad Network Providers` block documenting:

```markdown
## House Ads (fill tier)

`ad_campaigns.is_house` (boolean, default false) marks house campaigns.
Serving is two-pass: paid tier (`is_house=false`) is evaluated first; when no
eligible paid campaign matches, the house tier (`is_house=true`) fills the
slot. House campaigns are admin-only (`organization_id` must be NULL —
self-serve never sets `is_house`), require no budget, and may be created
directly in `active` status (admin-authored, no review gate).

## Serving Targeting

`GET /ads/serve?slot=<name>&space_id=<uuid>` enforces `target_geo` (ISO codes)
against `visitor.FromContext(ctx).ContentCountry` (GeoIP + profile, ADR-064a)
and `target_space_ids` against the optional `space_id` param. Empty target
arrays = untargeted. Visitors with no resolved country only see untargeted
campaigns. Event type vocabulary: `impression`, `click`, `conversion`.

## Additional Slot Types

`left_rail`, `article_inline`, `article_below_comments`, `comments_below`,
`native_feed`, `native_discover`, `native_space_feed`, `native_forum`,
`sticky_anchor`, `sidebar_sticky` — seeded in migration 000812.

## Ad Network Providers

`ad_networks.provider` CHECK expanded to 25 values: adsense,
google_admanager, ezoic, media_net, mediavine, raptive, freestar, setupad,
amazon_aps, prebid, magnite, index_exchange, pubmatic, openx, sovrn, criteo,
monumetric, she_media, carbon, buysellads, snigel, publift, adpushup,
valueimpression, custom. The frontend AdvertisementBlock is config-driven
(`config.script_url`, `config.attrs`, `config.placement_id`) so new providers
need no bespoke frontend code.
```

- [ ] **Step 2: Amend §61** — add under "Payment & Escrow":

```markdown
## Spend Accrual

`ad:accrue_spend_daily` (cron `0 4 * * *`, default queue) increments
`ad_campaigns.spent += flat_rate` once per active day. When
`spent >= budget_credits` the campaign transitions to `ended` and
`ads:refund_campaign_credits` refunds any remainder. Admin campaigns with
`budget > 0` auto-end at `spent >= budget`. `end_date < now()` also ends
the campaign on the same sweep.
```

- [ ] **Step 3: §11 + asynq-tasks.md** — add the catalog row:

```markdown
ad:accrue_spend_daily             — accrue flat-rate spend, auto-end exhausted/expired campaigns (Section 25)
```

and to `conventions/asynq-tasks.md` table: `| ad:accrue_spend_daily | Daily spend accrual + auto-end for flat-rate ad campaigns | Yes (cron) |`.

- [ ] **Step 4: api-routes-catalog.md** — update the Ads Serving block:

```markdown
## Ads Serving (Section 25 — public)
GET    /ads/serve?slot={name}&space_id={uuid?}   [public, optional auth]
POST   /ads/events                             [public, optional auth] event_type ∈ impression|click|conversion
GET    /ads/networks/config                    [public] active ad networks (api_key redacted)
```

and in the admin campaigns section note `is_house` (bool) on
`POST /admin/ads/campaigns` and `PATCH /admin/ads/campaigns/{id}`.

- [ ] **Step 5: system-settings-catalog.md** — add (all three must also land in `internal/control/settings_catalog.go` with `Public: true`, else `/site/settings/public` filters them out and `page.data.siteSettings` never sees them — that's Task 7 Step 3b):

```markdown
| `ads.native_feed_interval` | int | `5` | Insert a native ad after every N feed items (0 disables). |
| `ads.in_article_after_paragraph` | int | `3` | Paragraph index for first in-article ad. |
| `ads.in_article_every` | int | `6` | Repeat in-article ad every N paragraphs (0 = once). |
```

- [ ] **Step 6: MIGRATION-ORDER.md + ROADMAP-BACKEND.md** — add `000812_ads_completion` entry in order; mark backend roadmap task 24.05 (`ad:ingest_event_batch` → daily rollup) `[x]` with a note that rollup is now upserted at ingest time.

- [ ] **Step 7: Commit**

```bash
git add .jeos/
git commit -m "docs(jeos): ads completion — house tier, targeting, new slots, providers, spend accrual"
```

---

### Task 2: Migration 000812

**Files:**
- Create: `migrations/000812_ads_completion.up.sql`
- Create: `migrations/000812_ads_completion.down.sql`

- [ ] **Step 1: Write the up migration**

```sql
-- 000812_ads_completion.up.sql — ads system completion (§25, design spec
-- docs/superpowers/specs/2026-10-08-ads-system-completion-design.md)

-- 1. House-ad flag + serving index.
ALTER TABLE ad_campaigns ADD COLUMN is_house boolean NOT NULL DEFAULT false;
CREATE INDEX idx_ad_campaigns_house ON ad_campaigns (slot_type_id, status) WHERE is_house;

-- 2. Widen provider enum to 25 values.
ALTER TABLE ad_networks DROP CONSTRAINT ad_networks_provider_check;
ALTER TABLE ad_networks ADD CONSTRAINT ad_networks_provider_check CHECK (provider = ANY (ARRAY[
  'adsense','google_admanager','ezoic','media_net','mediavine','raptive',
  'freestar','setupad','amazon_aps','prebid','magnite','index_exchange',
  'pubmatic','openx','sovrn','criteo','monumetric','she_media','carbon',
  'buysellads','snigel','publift','adpushup','valueimpression','custom'
]));

-- 3. Conversion events.
ALTER TABLE ad_events DROP CONSTRAINT ad_events_event_type_check;
ALTER TABLE ad_events ADD CONSTRAINT ad_events_event_type_check CHECK (event_type = ANY (ARRAY['impression','click','conversion']));

-- 4. New slot types.
INSERT INTO ad_slot_types (slot_name, description, max_display_per_page) VALUES
  ('left_rail',              'Skyscraper ad in the collapsed-nav left rail', 1),
  ('article_inline',         'In-article ad injected inside the body after N paragraphs', 2),
  ('article_below_comments', 'Ad below the comments section on detail pages', 1),
  ('comments_below',         'Ad directly under comment threads', 1),
  ('native_feed',            'Native card-styled ad in the main feed', 2),
  ('native_discover',        'Native card-styled ad in the Discover grid', 2),
  ('native_space_feed',      'Native card-styled ad in space feeds', 2),
  ('native_forum',           'Native ad between forum thread rows', 2),
  ('sticky_anchor',          'Dismissible sticky banner anchored to the viewport bottom', 1),
  ('sidebar_sticky',         'Sidebar ad that remains visible while scrolling', 1)
ON CONFLICT DO NOTHING;

-- 5. Canonicalize legacy widget slot name.
UPDATE sidebar_widgets SET props = jsonb_set(props, '{slot}', '"sidebar"')
WHERE widget_type = 'ad_slot' AND props->>'slot' = 'sidebar_top';
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000812_ads_completion.down.sql

UPDATE sidebar_widgets SET props = jsonb_set(props, '{slot}', '"sidebar_top"')
WHERE widget_type = 'ad_slot' AND props->>'slot' = 'sidebar';

DELETE FROM ad_slot_types WHERE slot_name IN (
  'left_rail','article_inline','article_below_comments','comments_below',
  'native_feed','native_discover','native_space_feed','native_forum',
  'sticky_anchor','sidebar_sticky'
) AND id NOT IN (SELECT DISTINCT slot_type_id FROM ad_campaigns);

ALTER TABLE ad_events DROP CONSTRAINT ad_events_event_type_check;
ALTER TABLE ad_events ADD CONSTRAINT ad_events_event_type_check CHECK (event_type = ANY (ARRAY['impression','click']));

ALTER TABLE ad_networks DROP CONSTRAINT ad_networks_provider_check;
ALTER TABLE ad_networks ADD CONSTRAINT ad_networks_provider_check CHECK (provider = ANY (ARRAY['adsense','ezoic','media_net','custom']));

DROP INDEX IF EXISTS idx_ad_campaigns_house;
ALTER TABLE ad_campaigns DROP COLUMN is_house;
```

- [ ] **Step 3: Apply + verify**

```bash
make migrate   # or: go run ./cmd/migrate/
psql $DATABASE_URL -c "SELECT slot_name FROM ad_slot_types WHERE slot_name='left_rail';"
```

Expected: migration applies; `left_rail` row exists.

- [ ] **Step 4: Commit**

```bash
git add migrations/000812_ads_completion.*.sql
git commit -m "feat(migrations): 000812 ads completion — house flag, 25 providers, conversion events, new slots"
```

---

### Task 3: sqlc queries + regen

**Files:**
- Modify: `internal/db/queries/advertising.sql`
- Regenerate: `internal/db/gen/*` via `make sqlc-gen`

- [ ] **Step 1: Add serving + accrual queries to `advertising.sql`**

```sql
-- name: ListEligibleCampaignsForServing :many
-- Ref: sections/25-ad-placement.md (Serving Targeting, House tier)
SELECT * FROM ad_campaigns
WHERE slot_type_id = @slot_type_id
 AND status = 'active'
 AND is_house = @is_house
 AND (start_date IS NULL OR start_date <= now())
 AND (end_date IS NULL OR end_date >= now())
 AND (cardinality(target_geo) = 0 OR sqlc.narg(user_country)::text = ANY(target_geo))
 AND (cardinality(target_space_ids) = 0 OR sqlc.narg(space_id)::uuid = ANY(target_space_ids))
ORDER BY priority DESC, created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListActiveFlatRateCampaigns :many
-- For ad:accrue_spend_daily — all active campaigns that accrue daily spend.
SELECT * FROM ad_campaigns
WHERE status = 'active'
 AND (flat_rate > 0 OR end_date IS NOT NULL);

-- name: AccrueCampaignSpend :one
UPDATE ad_campaigns
SET spent = spent + flat_rate, updated_at = now()
WHERE id = @id AND flat_rate > 0
RETURNING *;
```

Also update the two campaign-write queries so `is_house` is persisted:

- `CreateAdCampaign` — add `is_house` to the column list with `@is_house` param.
- `UpdateAdCampaignDetails` — add `is_house = COALESCE(sqlc.narg(is_house), is_house)` to the SET clause.

And in `internal/db/queries/ads_self_serve.sql`, add a conversions count to `GetAdCampaignStats` (org-scoped):

```sql
 COUNT(*) FILTER (WHERE ae.event_type = 'conversion') AS conversions,
```

with a matching `Conversions int64` field on `adsselfserve.CampaignStats` (service.go:96) and the handler output.

`sqlc.narg` conventions (verified against generated code): `narg` on `text` → `*string` param (nil → NULL); `narg` on `uuid` → `pgtype.UUID` (invalid → NULL); `narg` on `bool` → `*bool`. The `= ANY(array)` form with NULL yields NULL → row excluded, which is exactly the targeting semantics wanted (country-less visitors only see untargeted campaigns).

- [ ] **Step 1b: `ad_analytics_daily` conflict target** — already verified: the baseline ships unique index `ad_analytics_daily_campaign_id_slot_type_id_date_key` on `(campaign_id, slot_type_id, date)`, so `UpsertAdAnalyticsDaily`'s `ON CONFLICT` works. No migration needed — this step is informational; remove nothing.

- [ ] **Step 2: Fix `GetCampaignStats` conversions** — replace `0::bigint AS conversions` with:

```sql
 COUNT(*) FILTER (WHERE e.event_type = 'conversion')::bigint AS conversions,
```

- [ ] **Step 3: Regenerate**

```bash
make sqlc-gen
go build ./internal/db/gen/ ./internal/advertising/
```

Expected: gen compiles; `ListEligibleCampaignsForServing`, `ListActiveFlatRateCampaigns`, `AccrueCampaignSpend` exist; `AdCampaign.IsHouse bool` field present.

- [ ] **Step 4: Commit**

```bash
git add internal/db/queries/advertising.sql internal/db/gen/
git commit -m "feat(db): targeting-aware serving query, spend accrual, conversion stats"
```

---

### Task 4: Serving — targeting + house tier + space_id

**Files:**
- Modify: `internal/advertising/service.go` (RenderSlot, CreateCampaign, CreateCampaignInput)
- Modify: `internal/advertising/store.go` (new store method)
- Modify: `internal/advertising/handlers.go` (RenderSlot space_id, create/update is_house)
- Test: `internal/advertising/advertising_test.go`

**Interfaces:**
- Produces: `Service.RenderSlot(ctx, userID pgtype.UUID, slotName, userCountry, spaceID string) ([]AdCampaign, error)`; `CreateCampaignInput.IsHouse bool`; wire key `is_house` on campaign JSON.

- [ ] **Step 1: Failing tests** — add table tests in `advertising_test.go` covering: paid campaign returned when eligible; house campaign returned only when paid tier empty; geo mismatch excludes campaign; space match includes / mismatch excludes; untargeted campaign serves to country-less visitor. (Tests follow the existing file's mock-Queries or service-test style — check how existing tests construct the store.)

- [ ] **Step 2: Store method**

```go
func (s *Store) listEligibleCampaigns(ctx context.Context, slotTypeID pgtype.UUID, userCountry, spaceID string, isHouse bool, limit int32) ([]AdCampaign, error) {
	var country *string
	if userCountry != "" {
		country = &userCountry
	}
	var space pgtype.UUID
	if spaceID != "" {
		_ = space.Scan(spaceID)
	}
	return s.queries.ListEligibleCampaignsForServing(ctx, gen.ListEligibleCampaignsForServingParams{
		SlotTypeID:  slotTypeID,
		IsHouse:     isHouse,
		UserCountry: country,
		SpaceID:     space,
		PageLimit:   limit,
		PageOffset:  0,
	})
}
```

Note: use `sqlc.narg` for `user_country`/`space_id` in the query so empty-string vs NULL semantics work — adjust the Task 3 SQL to `@user_country::text` → `sqlc.narg(user_country)::text` and same for `space_id`, OR pass sentinel values. Verify generated param types after regen and match them here.

- [ ] **Step 3: RenderSlot rewrite** (service.go:223) — replace `listActiveCampaignsBySlot` call:

```go
func (s *Service) RenderSlot(ctx context.Context, userID pgtype.UUID, slotName, userCountry, spaceID string) ([]AdCampaign, error) {
	slotType, err := s.store.Queries().GetAdSlotTypeByName(ctx, slotName)
	if err != nil {
		return []AdCampaign{}, nil // unknown slot → empty, never an error to the caller
	}
	campaigns, err := s.store.listEligibleCampaigns(ctx, slotType.ID, userCountry, spaceID, false, slotType.MaxDisplayPerPage)
	if err != nil {
		return nil, fmt.Errorf("listing eligible campaigns: %w", err)
	}
	if len(campaigns) == 0 {
		// House tier — fill unsold inventory.
		campaigns, err = s.store.listEligibleCampaigns(ctx, slotType.ID, userCountry, spaceID, true, slotType.MaxDisplayPerPage)
		if err != nil {
			return nil, fmt.Errorf("listing house campaigns: %w", err)
		}
	}
	if len(campaigns) == 0 {
		return []AdCampaign{}, nil
	}
	// … keep the existing frequency-cap filtering loop unchanged …
}
```

Note the behavior change: unknown slot name returns `[]` not an error — previously it 500'd on the frontend's fallback slots; now safe.

- [ ] **Step 4: Handler update** — `RenderSlot` handler parses `space_id` query param (validate UUID, ignore invalid → empty), pulls country:

```go
vc := visitor.FromContext(r.Context())
campaigns, err := h.Service.RenderSlot(r.Context(), userID, slotName, vc.ContentCountry, spaceID)
```

Import `github.com/jedafe/japa/internal/visitor`. Also `campaignToWire`: add `"is_house": c.IsHouse`.

- [ ] **Step 5: `is_house` on admin create/update** — add `IsHouse bool` to `CreateCampaignInput` + the handler's input struct (`json:"is_house"`); pass through to `gen.CreateAdCampaignParams{IsHouse: input.IsHouse}` (the sqlc change landed in Task 3 Step 1). House campaigns may be created directly in `active` — extend `CreateCampaign` to accept an optional `Status` override only when `IsHouse` (validate: `is_house` + explicit `pending_review` rejected; house skips the `ads.min_campaign_budget` check). Self-serve needs no change — `CreateSelfServeAdCampaign` never writes `is_house`, and the JSON input struct has no `is_house` field, so the flag is unreachable for orgs.

Update the PATCH handler input with `IsHouse *bool` and pass `sqlc.narg`-style nullable bool through `UpdateAdCampaignDetails` (check the generated param type — likely `pgtype.Bool`).

- [ ] **Step 6: Run tests + build**

```bash
go test ./internal/advertising/ -v && go build ./internal/advertising/
```

- [ ] **Step 7: Commit**

```bash
git commit -m "feat(ads): geo/space targeting + house fallback tier on /ads/serve"
```

---

### Task 5: Ingest worker — analytics rollup + conversion

**Files:**
- Modify: `internal/worker/handlers.go` (`HandleAdEventIngest` ~line 3605)
- Modify: `internal/advertising/handlers.go` (`IngestEvents` event_type validation ~line 537)
- Modify: `internal/advertising/service.go` (`IngestAdEvents` validation ~line 304)

- [ ] **Step 1: Accept `conversion`** — in `advertising.go` add `EventTypeConversion = "conversion"`; in `IngestEvents` + `IngestAdEvents` change the validation to `event_type IN (impression, click, conversion)`.

- [ ] **Step 2: Rollup in `HandleAdEventIngest`** — after `InsertAdEvent` succeeds:

```go
// Ref: sections/25-ad-placement.md — daily rollup written at ingest time.
// Resolve slot_type_id from the campaign; rollup failures are logged, never fatal.
campaign, err := queries.GetAdCampaignByID(ctx, campaignID)
if err != nil {
	slog.Warn("task:ad:ingest_event_batch: campaign lookup for rollup failed", "campaign_id", payload.CampaignID, "error", err)
	return nil // event is stored; rollup is best-effort
}
impressions, clicks := int32(0), int32(0)
switch payload.EventType {
case "impression":
	impressions = 1
case "click":
	clicks = 1
}
if payload.EventType == "impression" || payload.EventType == "click" {
	if _, err := queries.UpsertAdAnalyticsDaily(ctx, gen.UpsertAdAnalyticsDailyParams{
		CampaignID:  campaignID,
		SlotTypeID:  campaign.SlotTypeID,
		Date:        pgtype.Date{Time: time.Now().UTC(), Valid: true},
		Impressions: impressions,
		Clicks:      clicks,
		Spent:       0,
	}); err != nil {
		slog.Warn("task:ad:ingest_event_batch: rollup upsert failed", "campaign_id", payload.CampaignID, "error", err)
	}
}
```

(`ad_analytics_daily` has no `conversions` column — conversion events stay in `ad_events` only and are counted by `GetCampaignStats`.)

- [ ] **Step 3: Test** — extend/verify worker test: ingest an impression → `ad_analytics_daily` row exists with impressions=1; second impression increments to 2.

- [ ] **Step 4: Commit** — `git commit -m "feat(ads): daily analytics rollup at ingest + conversion event type"`

---

### Task 6: Spend accrual task `ad:accrue_spend_daily`

**Files:**
- Modify: `internal/worker/tasks.go` (type + registration ~line 97, 313)
- Modify: `internal/worker/enqueuer.go` (validTaskTypes ~line 224, tier map, cron table ~line 451)
- Modify: `internal/worker/handlers.go` (new `HandleAdsAccrueSpend`)
- Modify: `internal/worker/server.go` (`schedule(...)` in `NewScheduler`)

- [ ] **Step 1: Task type + registration**

```go
TypeAdsAccrueSpend = "ad:accrue_spend_daily"
```

Add `{TypeAdsAccrueSpend, HandleAdsAccrueSpend}` to the handler table in `tasks.go`, `TypeAdsAccrueSpend` to `validTaskTypes` in `enqueuer.go`, the default-tier loop entry, and the descriptive row near line 451: `{TypeAdsAccrueSpend, "default", "Accrue flat-rate spend and auto-end exhausted/expired ad campaigns (§25)"}`.

- [ ] **Step 2: Handler**

```go
// HandleAdsAccrueSpend accrues flat-rate daily spend and auto-ends campaigns
// that are exhausted or past end_date. Daily cron (§25 spend accrual).
func HandleAdsAccrueSpend(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:ad:accrue_spend_daily")
	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:ad:accrue_spend_daily: no database, skipping", "error", err)
		return nil
	}
	campaigns, err := queries.ListActiveFlatRateCampaigns(ctx)
	if err != nil {
		return fmt.Errorf("task:ad:accrue_spend_daily: %w", err)
	}
	now := time.Now().UTC()
	for _, c := range campaigns {
		updated, err := queries.AccrueCampaignSpend(ctx, c.ID)
		if err != nil {
			slog.Warn("task:ad:accrue_spend_daily: accrual failed", "campaign_id", c.ID.String(), "error", err)
			continue
		}
		// Record the accrued spend in the daily rollup so
		// ad_analytics_daily.spent reflects real daily spend (applies to
		// every accruing campaign, not just ended ones).
		// NOTE: only call this when updated.FlatRate > 0 — AccrueCampaignSpend
		// is a no-op (RETURNING the row unchanged) for flat_rate=0 campaigns,
		// which ListActiveFlatRateCampaigns may return for end_date sweeps.
		if updated.FlatRate > 0 {
			_, _ = queries.UpsertAdAnalyticsDaily(ctx, gen.UpsertAdAnalyticsDailyParams{
				CampaignID: c.ID, SlotTypeID: updated.SlotTypeID,
				Date: pgtype.Date{Time: now, Valid: true},
				Impressions: 0, Clicks: 0, Spent: int64(updated.FlatRate),
			})
		}
		expired := updated.EndDate.Valid && updated.EndDate.Time.Before(now)
		exhausted := (updated.BudgetCredits > 0 && updated.Spent >= int64(updated.BudgetCredits)) ||
			(updated.Budget != nil && updated.Spent >= *updated.Budget)
		if expired || exhausted {
			if _, err := queries.UpdateAdCampaignStatus(ctx, gen.UpdateAdCampaignStatusParams{ID: c.ID, Status: "ended"}); err != nil {
				slog.Warn("task:ad:accrue_spend_daily: end failed", "campaign_id", c.ID.String(), "error", err)
				continue
			}
			// Self-serve campaigns refund unused escrow via the existing task.
			if updated.OrganizationID.Valid {
				payload, _ := json.Marshal(map[string]string{"campaign_id": c.ID.String()})
				if taskEnqueuer != nil {
					// Enqueuer.Enqueue(taskType string, payload []byte) — queue
					// tier is resolved internally via QueueForTask.
					_ = taskEnqueuer.Enqueue(TypeAdsRefundCampaignCredits, payload)
				}
			}
		}
	}
	return nil
}
```

(Verified: worker handlers use the package-level `taskEnqueuer *Enqueuer` var — `Enqueue(taskType string, payload []byte, opts ...interface{})`. Never construct `asynq.NewTask` inside handlers.)

- [ ] **Step 3: Scheduler registration** in `NewScheduler`, with the other daily entries:

```go
// §25: flat-rate ad spend accrual + auto-end — daily at 04:00 UTC.
schedule("0 4 * * *", TypeAdsAccrueSpend, 24*time.Hour)
```

- [ ] **Step 4: Test** — unit test the handler against a fixture campaign (flat_rate=100, budget_credits=100): after one run spent=100 and status='ended'; a second campaign with end_date in the past also ends. Follow existing worker test patterns.

- [ ] **Step 5: Commit** — `git commit -m "feat(worker): ad:accrue_spend_daily — flat-rate accrual + auto-end"`

---

### Task 7: Provider enum + flag default + widget defaults

**Files:**
- Modify: `internal/advertising/service.go:509` (`validProviders`)
- Modify: `internal/features/registry.go:57` (`Default: false → true`)
- Modify: `internal/sidebars/widget_types.go:259` (`"sidebar_top"` → `"sidebar"`)

- [ ] **Step 1: `validProviders`** — replace the 4-entry map with all 25 (same list as the migration CHECK, alphabetical).

- [ ] **Step 2: `feature.ads` `Default: true`.**

- [ ] **Step 3: widget default** — `"slot": "sidebar"` in `ad_slot` widget `DefaultProps`; check `academy`/`docs` `AdSlot`/`LeftRail` fallbacks — LeftRail's `'left_rail'` fallback is now a real slot, keep it.

- [ ] **Step 3b: settings catalog** — add the three new keys to `internal/control/settings_catalog.go` in the `// Self-Serve Advertising` block, each with `Public: true` (required — `/site/settings/public` only returns `is_public` rows and `is_public` is driven by this catalog flag; without it `page.data.siteSettings` never receives them). Keep `Panel: "control"`:

```go
"ads.native_feed_interval":        {Type: "int", DefaultValue: "5", Public: true, Description: "Insert a native ad after every N feed items (0 disables).", Panel: "control"},
"ads.in_article_after_paragraph":  {Type: "int", DefaultValue: "3", Public: true, Description: "Paragraph index for the first in-article ad.", Panel: "control"},
"ads.in_article_every":            {Type: "int", DefaultValue: "6", Public: true, Description: "Repeat in-article ad every N paragraphs (0 = once).", Panel: "control"},
```

- [ ] **Step 4: Test + commit** — `go test ./internal/advertising/ ./internal/features/`; commit `feat(ads): 25-provider enum, feature.ads on by default, canonical widget slot names`.

---

### Task 8: Frontend `AdSlot` — space_id passthrough

**Files:**
- Modify: `apps/main/src/lib/components/ui/AdSlot.svelte`
- Modify: `apps/academy/src/lib/components/shell/AdSlot.svelte`
- Modify: `apps/docs/src/lib/components/shell/AdSlot.svelte`

- [ ] **Step 1:** Add `spaceId?: string` prop; append `&space_id=${encodeURIComponent(spaceId)}` to the `/ads/serve` URL when set. Same change in all three copies (they're per-app duplicates — diff them first to keep them in sync).

- [ ] **Step 2:** Pass `spaceId` where the shell knows the space — `spaces/[slug]` and `wiki/[space_slug]` pages that render sidebar widgets: check whether `SidebarWidgetRenderer` receives page context; thread `space_id` through the widget resolution only if trivially available. If the sidebar payload doesn't expose it, pass `spaceId` only on direct `AdSlot` mounts (wiki page detail slots) and leave sidebar widgets untargeted (space targeting then applies to direct mounts) — document the limitation in code.

- [ ] **Step 3: `bun run check`** in apps/main. Commit `feat(main): AdSlot space_id passthrough`.

---

### Task 9: `NativeAd` + feed injection

**Files:**
- Create: `apps/main/src/lib/components/ads/NativeAd.svelte`
- Modify: `apps/main/src/lib/components/feed/FeedList.svelte`
- Modify: `apps/main/src/lib/components/ui/index.ts` (or new `ads/index.ts` barrel)

- [ ] **Step 1: `NativeAd.svelte`** — same fetch/track logic as `AdSlot.svelte` (copy the `fetchAds`, `trackImpression`, `handleClick`, IntersectionObserver pattern; props `{slot: string, spaceId?: string}`), but renders the campaign as a feed-card-shaped block: image (if `content.image_url`) + headline + body + CTA chip + `Sponsored` label in muted text, wrapped in the same surface/border classes `FeedCard` uses. Hides entirely when no campaign.

- [ ] **Step 2: `FeedList` injection** — after the `{#each items}` loop structure, interleave:

```svelte
{#each items as item, i (item.id)}
	<FeedCard {item} style="daily" layout="stream" />
	{#if adInterval > 0 && (i + 1) % adInterval === 0 && i < items.length - 1}
		<NativeAd slot={adSlot} />
	{/if}
{/each}
```

with `const adInterval = $derived(Number(page.data.siteSettings?.['ads.native_feed_interval'] ?? 5))` and a new optional prop `adSlot?: string`. `max_display_per_page=2` caps repeats server-side anyway, but also guard: render `<NativeAd>` at most twice per list (track a counter or use `i/adInterval < 2`).

- [ ] **Step 3: Mount** — pass `adSlot="native_feed"` on the main feed page(s) that render `FeedList` (find the +page.svelte that uses it — `feedQueryString` callers: `/` feed, explore). `native_discover`/`native_space_feed`/`native_forum`: mount where discover grid, space feed, and forum thread lists render — locate each list component and apply the same interleave only if the loop is a simple `{#each}`; otherwise mount a single `<NativeAd>` after the Nth item.

- [ ] **Step 4:** `bun run check`. Commit `feat(main): native ad injection in feeds`.

---

### Task 10: `InArticleAd` + detail-page mounts

**Files:**
- Create: `apps/main/src/lib/components/ads/InArticleAd.svelte`
- Modify: `apps/main/src/routes/(app)/blog/[slug]/+page.svelte`
- Modify: wiki + content detail pages (locate: `wiki/[space_slug]`, story/content detail)

- [ ] **Step 1: `InArticleAd.svelte`** — props `{html: string, slot?: string = 'article_inline'}`. Client-side: parse `html` into block elements (`</p>`, `</h2>`, `</h3>`, `</figure>` boundaries), re-emit as an array of `{type:'html', frag}|{type:'ad'}` segments, injecting an ad marker after paragraph `after` (from `page.data.siteSettings['ads.in_article_after_paragraph'] ?? 3`) and every `every` paragraphs (`['ads.in_article_every'] ?? 6`, 0 = once). Render `{@html frag}` for html segments and `<AdSlot {slot} format="inline" />` for ad segments. SSR-safe: do the split in a `$derived` on `html` (string ops only, no DOM).

- [ ] **Step 2: Mount on blog page** — replace `{@html sanitizedBody}` region in `blog/[slug]/+page.svelte` with `<InArticleAd html={sanitizedBody} />` (keep the existing sanitize call upstream).

- [ ] **Step 3:** Same mount on wiki article body and content detail body — find the `{@html ...}` render site in each.

- [ ] **Step 4:** `bun run check`. Commit `feat(main): in-article ad injection on blog/wiki/content`.

---

### Task 11: `StickyAnchorAd`

**Files:**
- Create: `apps/main/src/lib/components/ads/StickyAnchorAd.svelte`
- Modify: `apps/main/src/routes/(app)/+layout.svelte`

- [ ] **Step 1:** Fixed bottom banner (`sticky_anchor` slot), session-dismissed via `sessionStorage['jc.ad.anchor.dismissed']`, close `×` button, `aria-label`, max-height ~90px, respects `prefers-reduced-motion`. Same fetch/track pattern as `AdSlot` (copy it — same `campaignToWire` JSON shape).

- [ ] **Step 2:** Mount once in `(app)/+layout.svelte` after the `AppShell` block, gated to authenticated layouts only (not `(auth)`/`(marketplace)` groups).

- [ ] **Step 3:** `bun run check`. Commit `feat(main): sticky anchor ad`.

---

### Task 12: Comments-ad mounts

**Files:**
- Modify: `apps/main/src/routes/(app)/blog/[slug]/+page.svelte` (below comments)
- Modify: content detail + forum thread pages

- [ ] **Step 1:** Mount `<AdSlot slot="comments_below" format="banner" />` directly under the comment thread component on blog, content detail, and forum thread pages; `<AdSlot slot="article_below_comments" format="banner" />` below the whole comments section on blog detail. Locate the comment components (`CommentThread` exists per build output) and add the slot after them.

- [ ] **Step 2:** `bun run check`. Commit `feat(main): comments-section ad placements`.

---

### Task 13: Menu `is_ad_slot` rendering

**Files:**
- Modify: `apps/main/src/lib/components/shell/LeftNav.svelte`
- Modify: `apps/main/src/lib/types/index.ts` (MenuItem already has `is_ad_slot`/`ad_slot_config` per types)

- [ ] **Step 1:** In the `{#each items}` loop, branch on `item.is_ad_slot`:

```svelte
{#if item.is_ad_slot}
	{#if !collapsed}
		{@const cfg = item.ad_slot_config ?? {}}
		<div class="my-2"><AdSlot slot={cfg.slot ?? 'sidebar'} format={cfg.format ?? 'square'} /></div>
	{/if}
{:else}
	<!-- existing anchor/NotificationCentre branch -->
{/if}
```

Remove `is_ad_slot` items from the `items` filter exclusion if present (check line ~43 `items` derived — currently filters visibility/feature only, so ad items already pass through; ensure `item.url` may be null on ad items and `isActive(item.url)`/`resolveIcon`/`routeFeatureFlagForPath` guard against null).

- [ ] **Step 2:** `bun run check`. Commit `feat(main): render is_ad_slot menu items as ad slots`.

---

### Task 14: Admin UI — house ads

**Files:**
- Modify: `apps/admin/src/routes/(app)/ads/new/+page.svelte` (house checkbox)
- Modify: `apps/admin/src/routes/(app)/ads/+page.svelte` (badge + filter)

(`adsAdminApi.createCampaign`/`updateCampaign` in `apps/admin/src/lib/api.ts` take `Record<string, unknown>` — no client signature change needed, just send `is_house` in the body.)

- [ ] **Step 1:** `/ads/new` — add `is_house` checkbox labeled "House ad (fallback fill)"; when checked, hide/disable the budget field and send `is_house: true` in the create body; send `status` field if the form already exposes one (house ads may be created active — check the form's status select; if absent, leave default `draft` + existing Activate action).

- [ ] **Step 2:** `/ads` list — add `house` chip on rows where `row.is_house` (same badge styling as status chips); add `'house'` to the status filter dropdown mapping to a client-side filter (`is_house === true`) since the backend `?status=` doesn't cover it; show `—` for budget on house rows.

- [ ] **Step 3:** `bun run check` in apps/admin. Commit `feat(admin): house ad create toggle + list badge/filter`.

---

### Task 15: `AdvertisementBlock` generic renderer

**Files:**
- Modify: `apps/main/src/lib/components/pagebuilder/blocks/AdvertisementBlock.svelte`

- [ ] **Step 1:** Widen `props.provider` to `string`. Replace the 4 hardcoded renderer branches with:

```ts
function injectScriptOnce(src: string) {
	if (!src || document.querySelector(`script[data-jc-ad-script="${src}"]`)) return;
	const s = document.createElement('script');
	s.src = src;
	s.async = true;
	s.dataset.jcAdScript = src;
	document.head.appendChild(s);
}
```

Render logic per provider:
- `adsense` → existing `ins.adsbygoogle` path (unchanged).
- `ezoic`, `media_net` → existing placeholder divs (unchanged).
- `prebid`, `amazon_aps`, `google_admanager` → `injectScriptOnce(config.script_url)` + placement div `data-placement={slotId}`.
- `custom` → `custom_html` (unchanged).
- everything else → if `network.config.script_url` → `injectScriptOnce` + div with `config.attrs` applied via `Object.entries(attrs).forEach(([k,v]) => div.setAttribute(k, String(v)))`; else placeholder div with `data-ad-provider`/`data-ad-slot`.

The `config` field already arrives on `AdNetwork.config` — type it `{ script_url?: string; attrs?: Record<string,string|number>; placement_id?: string }`.

- [ ] **Step 2:** `bun run check`. Commit `feat(main): config-driven ad-network renderer (25 providers)`.

---

### Task 16: Final verification

- [ ] **Step 1:** `go build ./... && go test ./internal/advertising/ ./internal/adsselfserve/ ./internal/worker/`
- [ ] **Step 2:** `bunx svelte-check` on apps/main + apps/admin (or `bun run check` per package.json).
- [ ] **Step 3:** Seed check — create a house campaign + paid campaign on the same slot via admin API; `GET /ads/serve?slot=…` returns paid; pause paid → returns house; geo-targeted campaign with `target_geo=['NG']` serves for NG visitor only.
- [ ] **Step 4:** Commit any fixes.

---

## Notes for the implementer

- `sqlc.narg` may be needed on `user_country`/`space_id` in `ListEligibleCampaignsForServing` — check generated param types after `make sqlc-gen` and reconcile Task 3 SQL with Task 4 store code before building.
- `ad_analytics_daily` upsert conflicts on `(campaign_id, slot_type_id, date)` — verify that unique constraint exists in the baseline (it's implied by the `ON CONFLICT` clause; if the constraint is missing, add it in 000812: `ALTER TABLE ad_analytics_daily ADD CONSTRAINT ad_analytics_daily_uniq UNIQUE (campaign_id, slot_type_id, date)` — check schema first).
- Academy (`apps/academy`) gets the new widget defaults automatically via the backend widget-type registry; no academy changes needed beyond the `AdSlot` spaceId prop if trivial.
