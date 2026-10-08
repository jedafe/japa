# Ads System Completion — Design

**Date:** 2026-10-08
**Status:** Approved approach
**Refs:** §25 (Ad Placement), §61 + ADR-079 (Self-Serve Ads), §11 (Asynq catalog),
§59 (Sidebars), ADR-060 (no ads on lesson player), ADR-018 (flat-rate pricing),
`internal/advertising`, `internal/adsselfserve`, `internal/visitor`, `internal/geoip`

## 1. Problem

The §25/§61 ads infrastructure is built end-to-end (slot-type registry, campaigns,
events, self-serve payment/review, external network config) but has six functional
gaps, no house-ad concept, and a provider list limited to four networks:

1. **`feature.ads` defaults to `false`** — serving is off unless ops toggles it.
2. **`target_geo` / `target_space_ids` are stored but never enforced** —
   `RenderSlot` only filters by slot + status + date window.
3. **`ad_analytics_daily` is never populated** — `UpsertAdAnalyticsDaily` has no
   callers; stats read raw `ad_events` rows.
4. **`spent` is never incremented** — `IncrementCampaignSpent` has no callers, so
   flat-rate self-serve campaigns never accrue spend and never auto-end on
   budget exhaustion.
5. **Slot-name mismatches** — sidebar widget defaults (`sidebar_top`,
   `left_rail`) are not seeded `ad_slot_types` names, so those placements fail
   `GetAdSlotTypeByName` and render nothing.
6. **`conversions` is hardcoded to 0** in `GetCampaignStats`; `menu_items.
   is_ad_slot` exists in schema + API but the only frontend consumer filters
   such items out instead of rendering them.
7. **No house ads** — admin campaigns (`organization_id IS NULL`) are implicit
   house ads with no dedicated flag or fill semantics.
8. **Only 4 ad-network providers** (`adsense`, `ezoic`, `media_net`, `custom`)
   and `AdvertisementBlock` hardcodes a renderer per provider.

## 2. Terminology

| Term | Meaning |
|---|---|
| **Paid tier** | Campaigns with `is_house = false` — admin-managed and self-serve. Served first. |
| **House tier** | Campaigns with `is_house = true` — admin-only, free, serves only when no paid campaign is eligible for a slot. |
| **Serve context** | `slot` (required), `space_id` (optional), visitor's resolved `ContentCountry` (from `internal/visitor` middleware). |
| **Native ad** | A campaign rendered by `NativeAd.svelte` styled like a feed card, labeled "Sponsored". |
| **In-article ad** | A campaign injected inside sanitized article HTML at a paragraph boundary by `InArticleAd.svelte`. |

## 3. Design

### 3.1 Gap fixes

**Feature flag.** `internal/features/registry.go`: `feature.ads` `Default: false → true`.

**Targeting.** `GET /ads/serve` accepts `?space_id=` (UUID, optional) in addition to
`?slot=`. `RenderSlot` resolves the visitor's country via
`visitor.FromContext(ctx).ContentCountry` — the visitor middleware is already mounted
upstream of the ads routes and enriches `ContentCountry` from GeoIP plus the signed-in
user's profile. When empty (no GeoIP DB, anonymous), only untargeted campaigns match.

New sqlc query `ListEligibleCampaignsForServing(slot_type_id, user_country, space_id, is_house)`:

```sql
SELECT * FROM ad_campaigns
WHERE slot_type_id = @slot_type_id
  AND status = 'active'
  AND is_house = @is_house
  AND (start_date IS NULL OR start_date <= now())
  AND (end_date   IS NULL OR end_date   >= now())
  AND (cardinality(target_geo) = 0 OR @user_country::text = ANY(target_geo))
  AND (cardinality(target_space_ids) = 0 OR @space_id::uuid = ANY(target_space_ids))
ORDER BY priority DESC, created_at ASC
LIMIT @page_limit OFFSET @page_offset;
```

Frequency capping stays in the service layer (per-user/day, unchanged).

**Daily analytics rollup.** The `ad:ingest_event_batch` worker handler upserts
`ad_analytics_daily` per event: after `InsertAdEvent`, call
`UpsertAdAnalyticsDaily(campaign_id, slot_type_id, occurred_at::date, +1 impression|click)`.
`slot_type_id` is resolved by joining `ad_campaigns` (the generated upsert already
`ON CONFLICT ... DO UPDATE SET impressions += EXCLUDED.impressions`). Rollup failures
are logged, never fail the task.

**Spend accrual.** New Asynq task `ad:accrue_spend_daily` (registered in §11 catalog,
cron `0 0 * * *` daily, `default` queue). For every `status='active'` campaign with
`flat_rate > 0`: `spent += flat_rate`. Then auto-transition:

- `spent >= budget_credits` (self-serve, `budget_credits > 0`) or `spent >= budget`
  (admin, `budget > 0`) → `status='ended'`; self-serve campaigns additionally
  enqueue `ads:refund_campaign_credits` for any unused remainder.
- `end_date < now()` → `status='ended'` (same refund path).

This implements ADR-018's flat-rate model: a campaign consumes `flat_rate` credits
per day it is active.

**Slot-name fixes.** Seed `left_rail` as a real slot type. Widget defaults change:
`ad_slot` widget `slot` prop default `sidebar_top` → `sidebar`; `LeftRail` fallback
`left_rail` stays (now seeded). Migration UPDATE rewrites existing
`sidebar_widgets.props->>'slot' = 'sidebar_top'` to `'sidebar'`.

**Conversion events.** Migration widens `ad_events_event_type_check` to include
`'conversion'`; ingest validation + `IngestAdEvents` accept it; `GetCampaignStats`
replaces `0::bigint AS conversions` with a `FILTER` count; self-serve `CampaignStats`
exposes `conversions`.

**Menu ad slots.** `LeftNav.svelte` renders `menu_items` where `is_ad_slot=true`
as `<AdSlot>` driven by `ad_slot_config` (`{slot, format}`) instead of being
filtered out; `dashboardViews.ts` keeps filtering them for hub-card views
(cards can't host ads).

### 3.2 House ads — fallback fill tier

- **Schema**: `ad_campaigns.is_house boolean NOT NULL DEFAULT false`; partial index
  `idx_ad_campaigns_house ON ad_campaigns (slot_type_id, status) WHERE is_house`.
- **Serving**: `RenderSlot` runs `ListEligibleCampaignsForServing(..., is_house=false)`;
  if empty, reruns with `is_house=true`. Frequency capping applies to both tiers.
- **Constraints**: `is_house` requires `organization_id IS NULL` — enforced in
  `Service.CreateCampaign` (reject `is_house && organization_id != nil`) and
  self-serve `CreateCampaign` never sets it.
- **Admin API**: `CreateCampaign`/`UpdateCampaign` accept `is_house`. House campaigns
  may be created directly in `active` (no review gate — they are admin-authored).
- **Admin UI**: `/ads/new` gains an "House ad" checkbox (disables budget fields);
  `/ads` list shows a house badge and a `house` filter value; house rows show
  "—" for budget.

### 3.3 New ad spaces

New `ad_slot_types` seeds (all `is_active=true`, `flat_rate=100` default):

| slot_name | max/page | Placement |
|---|---|---|
| `left_rail` | 1 | 160×600 skyscraper, collapsed-nav left rail (already rendered) |
| `article_inline` | 2 | Inside blog/wiki/content body after paragraph N |
| `article_below_comments` | 1 | Below comments on detail pages |
| `comments_below` | 1 | Under comment threads (blog, content, forum) |
| `native_feed` | 2 | Card-styled insertion in main feed |
| `native_discover` | 2 | Discover grid |
| `native_space_feed` | 2 | Space-scoped feeds |
| `native_forum` | 2 | Forum thread list |
| `sticky_anchor` | 1 | Dismissible bottom viewport banner |
| `sidebar_sticky` | 1 | Sidebar widget that sticks while scrolling |

New frontend components (`apps/main/src/lib/components/ads/`):

- **`InArticleAd.svelte`** — props `{html, slot='article_inline', after=3, every=6}`;
  splits sanitized HTML on `</p>`/`</h2>`/`</h3>` block boundaries and mounts an
  `AdSlot` at each injection point. Mounted on `blog/[slug]`; wiki + content
  detail reuse it.
- **`NativeAd.svelte`** — fetches `/ads/serve?slot=<native_*>` itself, renders the
  top campaign as a feed-card-shaped creative (headline, body, image, CTA,
  "Sponsored" badge), same impression/click tracking as `AdSlot`.
- **`StickyAnchorAd.svelte`** — bottom-fixed banner, `sticky_anchor` slot,
  close button persists dismissal in `sessionStorage` (`jc.ad.anchor.dismissed`).
- **`AdListInjector`** pattern for feeds: `FeedList` accepts an optional
  `adSlot` + `interval` and interleaves `<NativeAd>` every N items; interval from
  `ads.native_feed_interval` system setting (default 5), exposed to the page via
  site settings payload.
- **`StickyAnchorAd`** mounted once in `(app)/+layout.svelte`.
- **`space_id` pass-through**: `AdSlot`/`NativeAd` accept optional `spaceId`;
  space pages (`spaces/[slug]`, `wiki/[space_slug]`) pass their space UUID.

### 3.4 Ad network expansion

`ad_networks_provider_check` widened to 25 providers:

```
adsense, google_admanager, ezoic, media_net, mediavine, raptive, freestar,
setupad, amazon_aps, prebid, magnite, index_exchange, pubmatic, openx, sovrn,
criteo, monumetric, she_media, carbon, buysellads, snigel, publift, adpushup,
valueimpression, custom
```

`validProviders` in `internal/advertising/service.go` updated to the same list —
`/admin/ads/networks/providers` and the admin `<select>` pick it up automatically.

`AdvertisementBlock.svelte` is generalized to a config-driven renderer:

- `config.script_url` (string) → injected once per page (dedup via a
  `data-jc-ad-script` attribute).
- `config.placement_id` / `config.attrs` (object) → applied to the placement div.
- Special cases retained for `adsense` (`ins.adsbygoogle` + `adsbygoogle.push`),
  `ezoic` (`data-ezoic-id`), `media_net`, `prebid`/`amazon_aps`/`google_admanager`
  (header-bidding loader: script_url + `data-placement` div).
- `custom` keeps `custom_html` passthrough; all other providers fall through to
  the generic renderer — no per-provider frontend code needed.

### 3.5 JEOS updates (before code)

- `sections/25-ad-placement.md` — house tier, targeting enforcement, new slot
  types, expanded provider enum, conversion event, spend accrual model.
- `sections/61-self-serve-ads.md` — spent accrual + auto-end semantics.
- `sections/11-job-queue-asynq.md` + `conventions/asynq-tasks.md` — `ad:accrue_spend_daily`.
- `conventions/api-routes-catalog.md` — `?space_id=` on `/ads/serve`, `is_house`
  on admin campaign create, `conversion` event type.
- `conventions/system-settings-catalog.md` — `ads.native_feed_interval` (default 5).
- `internal/control/settings_catalog.go` — same key.
- `schemas/MIGRATION-ORDER.md`, `roadmaps/ROADMAP-BACKEND.md` — new entries; mark
  24.05 done (rollup now populated).

## 4. Migration plan

`000814_ads_completion.up/down.sql` — single pair:

1. `ALTER TABLE ad_campaigns ADD COLUMN is_house boolean NOT NULL DEFAULT false` + partial index.
2. Widen `ad_networks_provider_check` (drop + re-add CHECK with 25 values).
3. Widen `ad_events_event_type_check` to include `'conversion'`.
4. Seed the 10 new `ad_slot_types` rows.
5. Rewrite `sidebar_widgets.props->>'slot' = 'sidebar_top'` → `'sidebar'`.

Down: drop column/index, restore both CHECKs, delete seeded slots (only rows
with no referencing campaigns), revert widget-prop rewrite.

## 5. Testing

- `internal/advertising`: table tests for `RenderSlot` — paid-before-house,
  geo match/miss, space match/miss, untargeted fallback, frequency cap; provider
  validation for all 25 values.
- `internal/worker`: ingest task upserts `ad_analytics_daily`; accrual task
  increments spent, auto-ends at cap and at `end_date`, enqueues refund.
- `internal/adsselfserve`: `is_house` rejected; stats include conversions.
- Frontend: `bun run check` on touched apps; manual serve → render → event flow
  smoke test.

## 6. Non-goals

- IP-based frequency capping for guests (no stable user key).
- Advertiser-side conversion pixels/postbacks — API accepts `conversion` events;
  attribution wiring is a later concern.
- CPM/CPC pricing — flat-rate only per ADR-018.
- Per-user/per-cohort ad personalization beyond geo+space targeting.
- Admin UI redesign — `/ads` keeps its existing tabbed layout; only incremental
  additions (house badge/filter/toggle).
