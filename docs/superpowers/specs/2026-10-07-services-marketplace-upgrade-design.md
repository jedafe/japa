# Services & Packages Marketplace Upgrade — Design

Date: 2026-10-07
Status: proposed
Scope: `internal/servicedir`, `internal/coupons`, `internal/server`, `internal/catalog`, `internal/db`, `migrations`, `apps/main` (new `(marketplace)` route group), `apps/admin` (category admin optional).

## 1. Goal

JapaConnect becomes a hub connecting verified travel agents/agencies with travelers. The `/services` surface upgrades from a directory list to a full marketplace (reference: Japa.to mockups — hero, category pills, rich listing cards, tiered package pricing, reviews, FAQs), and listing creators (orgs) can attach coupon/discount codes to their listings — internal (platform-paid) and external listings.

Key user decisions (confirmed):

- Coupons give a **real discount at checkout** on internal `paid` listings; on external listings they are display/copy + referral click-through only.
- Visual language: mockup **structure/density**, JapaConnect **tokens** (`--color-*`, radius, fonts) — dark-mode safe.
- Chrome: dedicated slim **marketplace header** (logo, Services / Packages / Deals links, search, avatar) + marketing footer. Public pages leave the AppShell.
- Coupons are **org-owned**, managed in a **separate manage UI**, attachable to multiple listings.
- Categories become a **DB-managed dynamic taxonomy**.
- Packages surface via a **tab on `/services`** (existing `?kind=` contract), not a separate route.
- Coupon rules: **full-featured** (code, type, value, validity window, max redemptions, per-user limit, min spend, first-time-only, per-listing overrides, active/paused).
- **Package pricing tiers included** (Essential/Standard/Premium).
- Route scope: only `/services` and `/services/[slug]` leave the shell; `/services/new`, `/services/[slug]/edit`, `/me/service-bookings`, `/org/[slug]/service-bookings` stay inside `(app)`.

## 2. Backend

### 2.1 Dynamic service categories

- New table `service_categories`:
  - `id uuid pk`, `slug text unique`, `label text`, `description text`, `icon text` (lucide name), `accent_color text`, `applies_to text check in ('service','package','both')`, `position int`, `is_active bool`, `tenant_id`, timestamps.
  - Seed: existing 7 enum values (immigration_legal, relocation, finance, education, health, community, other) + marketplace additions: `study_abroad`, `work_abroad`, `travel`, `insurance`, `documents`, `family_sponsorship`.
- `service_listing_meta.service_category` remains `text`; validation in `servicedir.Service` reads the table (with in-process cache + Valkey `jc:service_categories` cache) instead of the hardcoded `categoryLabels` map. Unknown category → `invalid_category` error unchanged.
- `GET /services/categories` keeps its `{value, label}[]` contract (extended to include `icon`, `accent_color`, `applies_to`, `description` for the marketplace UI — additive fields, existing consumers unaffected).
- Admin CRUD: `GET/POST /admin/service-categories`, `PATCH/DELETE /admin/service-categories/:id` behind RBAC node `service_categories.manage` (seeded migration; granted to admin role). Slug is immutable after create (referenced by `service_listing_meta.service_category`); label/description/icon/color/position/active are mutable. Delete = soft (`is_active=false`) when listings reference the slug; hard-delete allowed otherwise.

### 2.2 Coupon ↔ listing link + full rule set

Extend the existing §55 `coupons` entity rather than building a parallel system:

- `ALTER TABLE coupons` adds: `max_redemptions int`, `per_user_limit int`, `min_spend bigint` (minor units), `first_time_only bool`, `applies_to text check in ('service','package','both')`, `organization_id uuid references organizations(id)` (org-owned coupons; existing `provider_id` remains for affiliate providers).
- New join `service_listing_coupons` (`id, tenant_id, coupon_id fk, listing_meta_id fk, position int, overrides jsonb` — overrides may re-scope `discount_value`/`min_spend` per listing; `unique(coupon_id, listing_meta_id)`).
- New `coupon_redemptions` (`id, coupon_id, user_id, service_booking_id, listing_meta_id, amount_saved bigint, currency, created_at`) — the authoritative record for cap/per-user/first-time checks. `coupon_events` (click/copy) unchanged.
- Org manage API: `GET/POST /organizations/:id/coupons`, `GET/PATCH/DELETE /organizations/:id/coupons/:cid`. Create validates code uniqueness per org, discount type+value, window, caps. Status transitions: active ⇄ suspended; expired is derived. AuthZ reuses the existing org-deals precedent (`/organizations/{id}/deals`): org admin/manager role on the organization.
- Attach API: `GET/PUT /services/:id/coupons` — set ordered coupon list for a listing; creator must own/manage the listing's org and own the coupons.
- Public surface: `GET /services/:slug` response gains `coupons: [{ id, code?, title, discount_type, discount_value, expires_at, masked }]`. Codes may be masked (`reveal_on: click|always`) — v1 shows the code (simpler; reveal tracking optional via existing `/coupons/:id/click`).

### 2.3 Checkout redemption (internal `paid` listings only)

`POST /services/:id/bookings` accepts optional `coupon_code`:

1. Load listing + attached active coupon matching code (case-insensitive), org-owned, applies_to covers listing kind, within validity window.
2. Validate: `max_redemptions` (count redemptions), `per_user_limit` (count by user), `first_time_only` (no prior bookings by user), `min_spend` vs `price_amount`.
3. Compute `discounted_total = price − discount` (percent or fixed, floor 0). Platform fee recomputed on `discounted_total` via existing `computeFee`. `agent_balance_due = discounted_total − fee`. The discount is **agent-funded**.
4. Snapshot on booking: existing `total_price`, `fee_amount`, `agent_balance_due` + new `coupon_id`, `coupon_code`, `discount_amount` columns on `service_bookings`.
5. Insert `coupon_redemptions` row inside the booking tx (atomic with fee-checkout creation; on payment abandonment the redemption stays bound to the pending booking and is released if the booking is cancelled — plan specifies release semantics).
6. Failure modes: `invalid_coupon` (not found/inactive/window), `coupon_not_applicable`, `coupon_cap_reached`, `coupon_user_limit`, `coupon_first_time_only`, `coupon_min_spend`. Codes are optional — absent code = current behavior.

New: `GET /services/:id/price-preview?coupon=&tier_id=` → `{ total, discount, fee, balance_due, currency, coupon_valid, error? }` for live UI preview (no mutation, rate-limited moderate).

External listings (`purchase_mode='external'`): no redemption; coupon renders as a copyable code card + `/deals`-style referral click-through tracking.

### 2.4 Package pricing tiers

- New table `service_package_tiers` (`id, tenant_id, listing_meta_id fk, name, badge text, description, price_amount bigint, price_currency, features text[] (checkmarks), position int, is_featured bool, is_active bool`).
- `GET /services/:slug` returns `tiers[]` for packages; card `From $X` = min active tier price (fallback `price_amount`).
- `POST /services/:id/bookings` accepts `tier_id`; snapshots `tier_id`, `tier_name`, `tier_price` on `service_bookings`. Fee computed on tier price (post-coupon).
- Create/edit payloads accept `tiers[]` (replace-set semantics, same pattern as package items/departures).
- Admin moderation unchanged (tiers ride the listing's review status).

### 2.5 Listing payload additions

- `featured_image_url` / `featured_media_id` surfaced from `contents.featured_image_id` on list + detail payloads (media join pattern matches existing queries).
- `rating_avg` + `review_count` on list rows (join the §29 reviews aggregate — same adapter used by `HandleGet`).
- `coupons[]` summary on detail; a `has_coupon`/best-discount badge field on list rows for card ribbons.
- Gallery beyond the featured image: **deferred to v2** (non-goal listed in §7).

### 2.6 Migrations (additive, zero-downtime)

1. `NNNNNN_service_categories.up/down.sql` — table + seed + indexes.
2. `NNNNNN_coupon_listing_link.up/down.sql` — coupons columns, `service_listing_coupons`, `coupon_redemptions`, RBAC node seed.
3. `NNNNNN_service_package_tiers.up/down.sql` — tiers table + `service_bookings` columns (`tier_id`, `coupon_id`, `discount_amount`, `coupon_code`).
4. sqlc regen after each; queries in `internal/db/queries/servicedir*.sql` + `coupons.sql`.

## 3. Frontend (`apps/main`)

### 3.1 Marketplace shell — `(marketplace)` route group

New 4th AppShell exception (documented in `jc-main-develop` skill + ADR note). Structure:

```
src/routes/(marketplace)/
  +layout.svelte        — MarketplaceHeader + <slot> + MarketplaceFooter
  +layout.server.ts     — session, features, nav data, csrfToken
  services/+page.*      — moved from (app)/services
  services/[slug]/+page.* — moved from (app)/services/[slug]
```

`/services/new`, `/services/[slug]/edit`, `/me/service-bookings`, `/org/[slug]/service-bookings`, `/org/[slug]/coupons` remain under `(app)`. SvelteKit merges routes across groups for matching (static `services/new` beats dynamic `services/[slug]`); the layout chain is per-directory, so `/services/new` keeps the shell while `/services/[slug]` gets marketplace chrome. Edge case: `/services/new/edit` would resolve to `(marketplace)/services/[slug]/edit` — not defined, so it 404s; acceptable since slug `new` is reserved on create.

`MarketplaceHeader`: logo → `/`, links Services (`/services`) / Packages (`/services?kind=package`) / Deals (`/deals`), global search input (→ `/services?q=`), notification bell + avatar → existing account dropdown. Slim height, `position: sticky`, surface + hairline border. `MarketplaceFooter`: link columns, newsletter slot, existing site footer style.

### 3.2 `/services` marketplace page

Layout (mockups 1–3 structure, JapaConnect tokens):

- Breadcrumb (Home / Services).
- Hero: headline, sub-copy, large search input + Search button; right side: value-prop list (Curated by Experts / Cost Savings / Hassle-free / Success Support — labels from `service_categories` benefits or i18n literals) — no hardcoded hex; accent via `--color-primary`.
- Category pills row: `All` + dynamic categories (icon + label, accent color chip), `More` overflow dropdown.
- Kind tabs: `Services | Packages` (writes `?kind=`); sort select (Popular/Latest/Top); results grid — 4-up cards on desktop.
- `ServiceCard` (new shared component): featured image (fallback category-tinted icon tile), status ribbon (`Popular`/`Best Value` — derived flags, not fake), wishlist heart (authed → bookmark API; guest → login redirect), title, excerpt, included-services checkmark chips (package items or top features), category pill, rating stars + count, `From $X` (min tier price), `View` CTA, coupon ribbon when `has_coupon`.
- Below grid: "Why book" trust strip, "Can't find / are you a provider" CTA band, provider strip (verified/orgs carousel optional — defer if no data source).
- Skeletons per section; `EmptyState` on zero results; load-error retry state (existing `loadError` contract preserved).

### 3.3 `/services/[slug]` detail page

Mockups 4–5 structure:

- Breadcrumb trail (Home / Services / Category / Title).
- Hero split: left — category badge, H1, description, star rating + review count + "N+ supported" stat, trust chips (Expert Guidance / High Success Rate / Secure & Reliable), CTA pair (`Get Started` → booking flow anchor, `Talk to an Expert` → inquiry/message); right — featured image card + fact grid (destination, level, processing time, intake dates).
- "What's Included" grid — icons + titles from package items / service features.
- "How It Works" numbered step rail.
- "Packages & Pricing" — tier cards (name, badge, price, one-time note, feature checklist, `Choose X` button, featured tier highlighted w/ "Most Popular" ribbon). Services (non-package) render a single price card.
- Coupon card (when attached): code, discount label, expiry, `Copy code` + terms link; external listings → `Go to provider` tracked outbound.
- Reviews section (existing §29 adapter data): featured review card + `View all reviews`.
- FAQ accordion (from listing `field_values`/faqs if present; otherwise hidden — no fake FAQs).
- Sticky bottom CTA band + org/provider card.
- Booking modal/drawer on `Choose tier`/`Get Started`: departure select (packages), party size, coupon code input + `Apply` → price-preview call → totals breakdown (total, −discount, fee due now, balance to agent) → submit `POST /services/:id/bookings` with `tier_id` + `coupon_code` + idempotency key.
- SEO: JSON-LD `Service`/`Product` + `AggregateRating` + `Offer`; `og` tags from featured image.

### 3.4 Authoring surfaces (stay in `(app)` shell)

- `/services/new` + `/services/[slug]/edit`: add featured-image picker (existing media picker), dynamic category select (from `/services/categories`), package tier editor (repeater: name/badge/price/features), coupon attach multi-select (from `GET /organizations/:id/coupons`).
- New `/org/[slug]/coupons` manage page (in shell): DataTable-spec list (code, discount, window, redemptions/caps, status, listings attached, actions) + create/edit drawer; stats chips (clicks, copies, redemptions).
- `/me/service-bookings` + org bookings: show applied coupon + discounted totals.

### 3.5 i18n / a11y / tokens

- All strings via `$t()` into `en.json` + `fr.json`.
- Colors only via CSS custom properties; category `accent_color` values constrained to a token-mapped palette list (admin picks from palette, not free hex).
- Keyboard focus visible, `prefers-reduced-motion` respected, cards are real links (a11y).

## 4. RBAC / settings / flags

- New permission nodes: `service_categories.manage` (admin), `coupons.create`/`coupons.manage.own` or reuse `organizations.manage` for org coupon CRUD — plan finalizes against `bootstrapPermissions`.
- Feature flags reused: `service_directory.enabled`, `service_packages.enabled`; new `service_coupons.enabled` + `service_packages.tiers_enabled` gates so the additions dark-launch safely.
- Settings: `service_packages.coupons.enabled`, `coupons.max_per_listing` (default 3).

## 5. Data flow summary

- List page SSR: `+page.server.ts` → `GET /services?kind=&category=&q=&dest=…` → cards. Filter bar writes URL params (SSR re-load) — same as current.
- Detail SSR: `GET /services/:slug` (now includes tiers, items, departures, coupons, rating aggregate, featured image).
- Booking: client POST with `Idempotency-Key`; fee checkout redirect/resume flow unchanged.
- Coupons manage: org-scoped fetch + mutations with CSRF token.

## 6. Testing

- Backend: sqlc-level store tests (testdb) for categories CRUD, coupon attach/validation, redemption caps/per-user/first-time/min-spend, tiered booking snapshots; handler tests for new routes; existing servicedir/coupons suites must stay green.
- Frontend: component tests for `ServiceCard` price/coupon ribbon variants; page-level SSR load tests; booking modal coupon flow test with mocked preview.
- Manual: responsive 360→1440, dark mode, `service_*` flag off behavior (routes 404 as today).

## 7. Non-goals (v1)

- Coupon stacking, auto-applied coupons, buyer-side coupon wallet.
- Agent payout/full-price collection — platform still collects fee only.
- Category hierarchy (flat list only).
- Gallery beyond featured image (deferred, see 2.5).
- Marketplace self-serve payments onboarding for agents.

## 8. Phasing / deliverables

| Phase | Deliverable | Depends on |
|---|---|---|
| A1 | Migrations + categories CRUD + API | — |
| A2 | Coupon columns + link + org manage API + attach API | A1 (parallel ok) |
| A3 | Tiers + booking redemption + price-preview | A2 |
| B1 | `(marketplace)` group + header/footer + `/services` redesign | A1 |
| B2 | `/services/[slug]` redesign + booking modal w/ coupon + tiers | A3 |
| C1 | Org coupon manage UI + form attach + tier editor + image picker | B1 |
| C2 | Admin category management UI (apps/admin) | A1 |
