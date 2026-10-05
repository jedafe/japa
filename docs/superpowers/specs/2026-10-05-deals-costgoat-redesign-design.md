# Deals Redesign — CostGoat-style index/detail + frontend deal management

**Date:** 2026-10-05
**Status:** Draft — awaiting user review
**Refs:** `.jeos/sections/55-coupons-external-affiliates.md`, costgoat.com/deals + costgoat.com/deals/1bench.dev (design references)

## Problem

`/deals` and `/deal/{provider}/{slug}` are thin card grids and a single detail card.
CostGoat's deal pages are richer: a scannable deal table with category filters, and a
long-form deal page (active offer, secondary offer, "why" sections, "ways to save",
FAQ) that reads like a proper landing page. There is also no way to create or edit a
deal from the main app — staff must use the admin panel.

## Decisions (from user)

- Deal creation is **staff-only**, gated on the existing `coupons.manage` node —
  no community submissions or moderation queue.
- Rich editorial content is stored on the coupon as a **`details` JSONB** column,
  with generated fallbacks so deals without editorial content still render well.
- **Category lives on `affiliate_providers`** (a provider is "Cloud" or "Dev"; all
  its deals inherit it). Internal coupons (no provider) are uncategorized.
- **Editing flows from the frontend as inline edit** — staff toggles edit mode on
  the deal detail page (and the provider landing page) instead of navigating to the
  admin panel.

## Backend

### Migration `000794_coupon_details_provider_category.up.sql`

```sql
ALTER TABLE affiliate_providers ADD COLUMN category text;
ALTER TABLE coupons              ADD COLUMN details  jsonb;
```

- `affiliate_providers.category` — free text, no CHECK constraint (categories
  evolve without migrations). Indexed? No — filtering happens in the frontend over
  the active-deal list (page_size 48 loads everything visible anyway).
- `coupons.details` — nullable JSONB, shape:

```json
{
  "summary":  "string — lede under the H1",
  "trial":    { "title": "...", "body": "...", "url": "..." },
  "sections": [{ "title": "...", "body": "..." }],
  "savings":  [{ "title": "...", "body": "..." }],
  "faq":      [{ "q": "...", "a": "..." }]
}
```

All keys optional. `details` is validated server-side as a JSON object capped at
32 KB; unknown keys are preserved (forward-compatible), rendering ignores them.

Down migration drops both columns.

### sqlc (`internal/db/queries/coupons.sql`) → `make sqlc-gen`

- `CreateAffiliateProvider`, `UpdateAffiliateProvider`, `GetAffiliateProviderByID/Slug`,
  `ListAffiliateProviders`: add `category`.
- `CreateCoupon`, `UpdateCoupon`: add `details` (`sqlc.narg(details)`).
- `GetCouponBySlug`, `GetCouponByID`, `ListCoupons`, `ListActiveCoupons`: select
  `c.details` and `ap.category AS provider_category`.
- New `ListActiveDealPartners :many` — `id, name, slug, description, category,
  logo_media_id, base_url, is_featured` from `affiliate_providers WHERE is_active`.

### Service/handlers (`internal/coupons`)

- `CreateProviderInput` / `UpdateProviderInput`: + `Category *string`.
- `CreateCouponInput` / `UpdateCouponInput`: + `Details json.RawMessage`.
  Validation: when non-empty must decode to a JSON object and be ≤ 32 KB →
  `400 validation_error` otherwise. Storage as `pgtype.JSONB`/`jsonb`.
- New handler `ListDealPartners` → `GET /api/v1/deals/partners` (public). Returns
  active providers — powers the provider select in the deal form and the category
  datalist (Rule 16: no hardcoded option lists).
- Cache: coupon mutations already SCAN-delete `jc:coupon_list:*` and per-slug/ID
  keys. Provider mutations must also flush `jc:coupon_list:*` + the affected
  provider's coupon caches since provider name/category now render inside coupon
  payloads — wire the same invalidation helper into `UpdateProvider`/`CreateProvider`.
  No new Valkey key patterns (catalog unchanged).
- Wire `r.Get("/deals/partners", couponHandlers.ListDealPartners)` in
  `internal/server/server.go` alongside the existing
  `/deals/partners/{slug}` route (chi treats the literal and param segments
  as distinct patterns — no ordering conflict).

### JEOS updates

- `.jeos/sections/55-coupons-external-affiliates.md` — schema block (new columns),
  API routes (+ `GET /deals/partners`, document `details`/`category` on payloads),
  Deal Pages subsection (new page structure + staff create/edit UX).
- `.jeos/conventions/api-routes-catalog.md` — add the new route.

## Frontend (`apps/main`)

### Types (`src/lib/types/index.ts`)

```ts
export interface CouponDetails {
  summary?: string;
  trial?: { title?: string; body?: string; url?: string };
  sections?: { title?: string; body?: string }[];
  savings?: { title?: string; body?: string }[];
  faq?: { q?: string; a?: string }[];
}
```

- `Coupon`: + `details?: CouponDetails | null`, `provider_category?: string | null`.
- `DealPartnerProvider`: + `category?: string | null`.
- New `DealPartnerRef` — slim provider row from `GET /deals/partners`.

### `/deals` — index redesign

- Header row: `ListPageBar` title + "Last updated {max(updated_at)}" + **New deal**
  button → `/deals/new`, rendered only when `page.data.user?.permissions` contains
  `coupons.manage`.
- Category pills: `All` + distinct `provider_category` values from loaded coupons.
  Client-side filter, synced to `?category=` (shareable). Uncategorized deals only
  appear under `All`. Replaces the partner strip (the table shows provider already).
- Table on `sm+` (Provider | Category | Deal | Details | Claim →), stacked rows on
  mobile. Row click → `dealPath(coupon)`. Category shown as a small pill; verified
  badge kept; `discountLabel` logic moved into the Deal column.
- Empty/load-error states unchanged.

### `/deal/{provider}/{slug}` — rich detail page

Sections in order (each skipped when its data is absent):

1. Breadcrumb (Deals › provider › title) + "Last updated {updated_at}".
2. H1 + `details.summary` (fallback: `description`).
3. **Active offer** card — provider name, description, mono `code` block + copy
   button when present, primary CTA → `/go` URL. Verified badge.
4. **Secondary offer** card (`details.trial`) — title, body, CTA to `trial.url`.
5. **"Why {provider}"** numbered list from `details.sections`; fallback when
   absent: a generated "How to redeem" 2–3 step block derived from
   code-vs-auto-apply (copy code / follow link / discount at checkout).
6. **"Every way to save on {provider}"** cards from `details.savings`.
7. **FAQ** accordion from `details.faq`; emits `FAQPage` JSON-LD in addition to the
   existing `Offer` JSON-LD.
8. Staff **Edit** button (`coupons.manage`) in the header → swaps the body for
   `<DealForm mode="edit">` inline (no navigation). Save → `PATCH /admin/coupons/{id}`
   → `invalidateAll()` back to view; Cancel → back to view.

### `/deal/{provider}` — partner landing

- Same card grid, plus staff **Edit provider** button (`affiliate.providers.manage`)
  → inline edit form (name, description, category, base_url, is_featured, is_active)
  → `PATCH /admin/affiliate-providers/{id}`.

### `/deals/new` — staff create page

- `+page.server.ts`: redirect `/deals` unless `permissions` includes
  `coupons.manage` (the `/ama/new` pattern); loads provider list via
  `GET /deals/partners`.
- Shared component **`DealForm.svelte`** (`src/lib/components/deals/`), also used
  by inline edit:
  - Provider `<select>` (from `/deals/partners`); "＋ New provider" toggle shown
    only with `affiliate.providers.manage` → inline mini-form (name, slug,
    category, base_url, description) → `POST /admin/affiliate-providers`, then
    selects the new provider. Category input uses a `<datalist>` fed by distinct
    categories from the provider list (Rule 16 — no hardcoded categories).
  - Coupon fields: title, slug (auto-slugify from title until manually touched),
    code (optional), discount_type/value/currency, referral_url, terms_url,
    description, valid_from, expires_at, is_verified, status.
  - Details editor: summary textarea; repeatable add/remove row editors for
    sections, savings, FAQ; trial field group. Builds the `details` object — omit
    the key entirely when all empty.
  - Create → `POST /admin/coupons` → `goto(dealPath(created))`.
    Edit → `PATCH /admin/coupons/{id}`.

### i18n

New keys in `en.json` + `fr.json` under `deals.*` / `deal.*` / `coupon.*`
(last_updated, new_deal, edit, categories, all_categories, form labels, section
headings, faq, trial, save/cancel, created/error toasts).

## Testing / verification

- `make sqlc-gen` compiles clean; `go build ./...`; `go test ./internal/coupons`.
- `bun run check` (svelte-check) in `apps/main`; `bun run lint` if configured.
- Manual: run backend + main app, create a deal end-to-end via `/deals/new`,
  verify table/filter/detail/FAQ render and inline edit round-trips.

## Out of scope

- Community deal submissions / moderation queue.
- Per-coupon category overrides (provider is the single source of category).
- Admin panel changes (its coupons page keeps working; it may optionally surface
  the new fields later).
