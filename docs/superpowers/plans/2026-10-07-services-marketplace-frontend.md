# Services Marketplace Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild `/services` and `/services/[slug]` as a full marketplace experience (Japa.to mockup structure on JapaConnect tokens) in a new `(marketplace)` route group that leaves the AppShell, consuming the backend surfaces shipped in the backend plan (dynamic categories, tiers, coupons, ratings, featured images, price preview).

**Architecture:** New `(marketplace)` route group with its own `+layout` (MarketplaceHeader/Footer). The two public pages move from `(app)/services` to `(marketplace)/services` — URLs unchanged. Authoring pages (`/services/new`, `/services/[slug]/edit`, bookings, org manage) stay in `(app)`. All data via SSR `+page.server.ts` → existing backend endpoints. New shared components under `src/lib/components/marketplace/`.

**Tech Stack:** SvelteKit 2 + Svelte 5 strict Runes, TypeScript, Tailwind v4 + CSS custom-property tokens, Bits UI (dot notation), Lucide Svelte, `$lib/i18n` `t()`, vitest.

## Global Constraints

- **Svelte 5 strict Runes** — `$state`, `$derived`, `$props`, `$effect`. Never: `export let`, `$:`, `on:click`, `createEventDispatcher`, localStorage for auth/UI state.
- **JEOS Rule 16** — no hardcoded data that belongs to the API. Categories, listings, tiers, coupons, reviews all come from endpoints. Static i18n *labels* are fine; lists of options are not.
- **API-Readiness Gate** — every endpoint consumed must exist in the backend route catalog (`internal/server/server.go`). The backend plan is already merged; verify each route before wiring (listed per task).
- **Tokens only** — colors via `var(--color-*)` CSS custom properties; `[data-theme="dark"]` handles dark mode — never Tailwind `dark:` classes, never hardcoded hex.
- **i18n** — every user-facing string via `t('key', 'fallback')`; add keys to `apps/main/src/lib/i18n/en.json` AND `fr.json`.
- **Bun** — `bun run`/`bunx` only (Rule 18). Verify with `bun run check` (svelte-check) and `bun run test` (vitest).
- **JEOS Rule 17** — no AI attribution anywhere.
- **Concurrent session** — another session commits to this repo frequently; stage ONLY your own files.
- **API base**: client calls go through `api` from `$lib/api/client` (browser) and `apiGet`/`apiList`/`apiGetEnvelope` from `$lib/server/api` (SSR).
- **Flags**: pages must degrade when flags are off — `service_directory.enabled` off → 404 (existing behavior); `service_packages.enabled` off → no Packages tab, no tier UI; `service_coupons.enabled` off → no coupon UI; `service_packages.tiers_enabled` off → no tier cards (booking uses base price).

## Existing assets to reuse (verify, don't rebuild)

- `$lib/components/ui/`: `EmptyState.svelte`, `LoadingSkeleton.svelte`, `PaginationControls.svelte`, `SearchInput.svelte`, `Modal.svelte`, `UrlTabs.svelte`, `SelectField.svelte`, `Autocomplete`-style pickers under `$lib/components/create/`.
- `formatMoney(minorUnits, currencyCode, currencies)` — `$lib/utils/currency.ts:103`; `currencies` array comes from root layout data.
- `$lib/enumOptions` — `serviceCategoryOptions`/`serviceCategoryLabel` are the OLD hardcoded enum; marketplace pages use API categories instead.
- `t` from `$lib/i18n` — signature `t('key', 'Fallback string')`.
- `api.post` with `Idempotency-Key` — existing booking POST pattern at `(app)/services/[slug]/+page.svelte:260-262`.
- `ServiceReview`, `ServiceReviewAggregate`, `PackageItem`, `PackageDeparture`, `ServiceBookingResult` types in `$lib/types/index.ts`.

## Backend contract (already shipped — verify, don't guess)

- `GET /services/categories` → `{data: [{value, label, description?, icon?, accent_color?, applies_to?}]}` (backend `Option` struct — `internal/servicedir/service.go` ~line 363; no `position` field in JSON)
- `GET /services?...` → items now carry `featured_image_url`, `featured_media_id`, `rating_avg`, `review_count`, `has_coupon`, `min_price` (all `omitempty`).
- `GET /services/{slug}` → `data` (ServiceListing + `tiers[]`, `coupons[]`) + sidecars `reviews[]`, `review_aggregate`.
- `GET /services/{id}/price-preview?tier_id=&coupon_code=` (auth, rate-limited) → `{data:{base,discount,total,fee,balance_due,currency,coupon_valid,error_code?}}`; invalid tier → 400 `invalid_tier`.
- `POST /services/{id}/bookings` — body gains `tier_id`, `coupon_code`; `Idempotency-Key` required for `intent='booking'`.
- Coupon error codes: `invalid_coupon, coupon_not_applicable, coupon_min_spend, coupon_cap_reached, coupon_user_limit, coupon_first_time_only, invalid_tier`.
- `ListingCoupon` JSON: `{id, title, code, discount_type, discount_value, currency, expires_at, terms_url, referral_url, applies_to}`.
- `PackageTier` JSON: `{id, name, badge, description, price_amount, price_currency, features[], position, is_featured}`.

---

### Task 1: Type contract + category helper

**Files:**
- Modify: `apps/main/src/lib/types/index.ts` (extend `ServiceListing`; add `ListingCoupon`, `PackageTier`, `ServicePricePreview`, `ServiceCategoryInfo`)
- Modify: `apps/main/src/lib/enumOptions.ts` (keep existing helpers; do NOT break form pages still using them)
- Create: `apps/main/src/lib/marketplace.ts` (`serviceCategoryInfo`, `priceLabel`, `discountLabel` helpers)
- Test: `apps/main/src/lib/marketplace.test.ts`

**Interfaces:**

```ts
// types/index.ts additions
export interface ServiceCategoryInfo {
	value: string;
	label: string;
	description?: string;
	icon?: string;        // lucide icon name
	accent_color?: string; // palette token name (violet|green|amber|blue|sky|emerald|rose|orange|slate|indigo|gray|cyan)
	applies_to?: 'service' | 'package' | 'both';
	// note: position is NOT serialized by the API (Option struct) — order comes
	// pre-sorted from the backend; do not add a client-side sort.
}

export interface ListingCoupon {
	id: string;
	title: string;
	code: string;
	discount_type: 'percent' | 'fixed' | string;
	discount_value: number;
	currency?: string;
	expires_at?: string;
	terms_url?: string;
	referral_url?: string;
	applies_to?: 'service' | 'package' | 'both';
}

export interface PackageTier {
	id: string;
	name: string;
	badge?: string;
	description?: string;
	price_amount: number; // minor units
	price_currency?: string;
	features: string[];
	position: number;
	is_featured: boolean;
}

export interface ServicePricePreview {
	base: number;
	discount: number;
	total: number;
	fee: number;
	balance_due: number;
	currency: string;
	coupon_valid: boolean;
	error_code?: string;
}

// ServiceListing gains (all optional/omitempty from API):
//   featured_image_url?: string;
//   featured_media_id?: string;
//   rating_avg?: number;
//   review_count?: number;
//   has_coupon?: boolean;
//   min_price?: number | null;   // minor units; falls back to price_amount server-side
//   tiers?: PackageTier[];
//   coupons?: ListingCoupon[];
```

```ts
// lib/marketplace.ts
// Map accent_color token names to CSS var-backed chip classes — palette is
// FIXED (admin palette constraint); unknown values fall back to 'slate'.
export type CategoryAccent = 'violet'|'green'|'amber'|'blue'|'sky'|'emerald'|'rose'|'orange'|'slate'|'indigo'|'gray'|'cyan';
export function accentClasses(accent: string | undefined): { chip: string; icon: string }; // e.g. 'cyan' → { chip: 'bg-cyan-500/10 text-cyan-700 …', icon: 'text-cyan-600' } — implement via a Record<CategoryAccent,...> lookup with slate fallback
export function serviceCategoryInfo(cats: ServiceCategoryInfo[], slug: string | undefined): ServiceCategoryInfo | undefined
export function priceLabel(listing: ServiceListing, currencies: CurrencyInfo[]): string | null
//   → min_price ?? price_amount → formatMoney; null when both absent
//   → suffix ' /person'|' /package'|' /group' by price_per via t() keys
export function discountLabel(c: ListingCoupon, currencies: CurrencyInfo[]): string
//   → percent: '-10%' ; fixed: '-' + formatMoney(value, currency); free_shipping unreachable but return ''
```

- [ ] **Step 1: failing test** `lib/marketplace.test.ts` — `priceLabel` (min_price preferred over price_amount, null when neither, per-suffix), `discountLabel` (percent vs fixed vs unknown), `accentClasses` (known → mapped, unknown → slate fallback).
- [ ] **Step 2:** `bun run test src/lib/marketplace.test.ts` — FAIL.
- [ ] **Step 3:** implement types + helpers.
- [ ] **Step 4:** test PASS; `bunx svelte-check` clean on touched files.
- [ ] **Step 5:** commit `feat(main): marketplace types + price/discount/category helpers`.

---

### Task 2: `(marketplace)` route group + chrome

**Files:**
- Create: `apps/main/src/routes/(marketplace)/+layout.svelte`, `+layout.server.ts`
- Create: `apps/main/src/lib/components/marketplace/MarketplaceHeader.svelte`
- Create: `apps/main/src/lib/components/marketplace/MarketplaceFooter.svelte`
- Move: `apps/main/src/routes/(app)/services/+page.{server.ts,svelte}` → `(marketplace)/services/` (`git mv`)
- Move: `apps/main/src/routes/(app)/services/[slug]/+page.{server.ts,svelte}` → `(marketplace)/services/[slug]/` (`git mv`). Do NOT move `[slug]/edit` or `new` — they stay in `(app)`.
- Modify: moved `+page.svelte` files minimally — only what won't compile outside the shell (check imports of `$lib/components/shell` or layout-provided props; the root layout already provides user/siteSettings/menu/footerMenu/features).

**Interfaces:**
- Consumes: root `+layout.server.ts` returns `{user, isAuthed, viewerId, siteSettings, footerMenu, menu, features, unreadNotifications, currencies, csrfToken, …}` — available via `data` prop in the group layout (parent chain).
- Produces: `MarketplaceHeader` props `{user, unreadCount, logoUrl?, logoDarkUrl?}`; `MarketplaceFooter` props `{menu?: Menu}`; layout passes these from root data.

```svelte
<!-- (marketplace)/+layout.server.ts -->
// Pass-through: root layout already loads everything the chrome needs.
// Keep this file to document the exception + reserve for marketplace-scoped
// data later; return {} so child pages still get parent() data.
export const load: LayoutServerLoad = async () => ({});
```

```svelte
<!-- +layout.svelte -->
<script lang="ts">
  // ADR-058 5th shell exception: public marketplace chrome (services
  // directory leaves the AppShell — spec §3.1).
  import MarketplaceHeader from '$lib/components/marketplace/MarketplaceHeader.svelte';
  import MarketplaceFooter from '$lib/components/marketplace/MarketplaceFooter.svelte';
  let { data, children } = $props();
</script>
<MarketplaceHeader user={data.user} unreadCount={data.unreadNotifications} logoUrl={...} logoDarkUrl={...} />
<main class="min-h-[60vh]">{@render children()}</main>
<MarketplaceFooter menu={data.footerMenu} />
```

**MarketplaceHeader requirements** (sticky, hairline border, `--color-surface` bg):
- Logo → `/` (use `data.siteSettings['appearance.logo_url']`/`_dark` like AuthShell does — check `AuthShell.svelte` for the exact key convention).
- Nav links: Services `/services`, Packages `/services?kind=package` (hide when `features['service_packages.enabled']` false), Deals `/deals`. Active state via `$page.url`.
- Search input (compact, expands on mobile via icon toggle is optional — keep a visible input ≥ `md:`): submits `goto('/services?q=' + term)`.
- Right side: if `user` — notifications link (bell icon + unread badge from `unreadNotifications`) → `/notifications`, avatar → existing account dropdown pattern (check how `(app)` shell renders the user menu — reuse the same component if it's extracted, else a Bits UI DropdownMenu with Profile/Settings/Bookings/Logout links matching existing destinations).
- Guest state: `Log in` → `/login?next=`, `Get started` → `/signup` buttons.
- Mobile: hamburger → slide-down nav with the same links; sticky top-0 z-40.

**MarketplaceFooter:** reuses `footerMenu` (admin-managed menu columns — check the `(app)` footer or root layout rendering of `footerMenu` for the item shape) + a newsletter slot ONLY if an existing newsletter component/endpoint exists (grep `newsletter` — if none, omit the slot entirely, no dead UI). Otherwise link columns + copyright line `© {year} {site name}` from siteSettings.

**Routing check (do this first):** SvelteKit will error on duplicate `/services` routes — the `git mv` must fully remove `(app)/services/+page.*`. Verify `bun run dev` boots and:
- `/services` renders inside marketplace chrome (no AppShell sidebars)
- `/services/new` still renders inside AppShell
- `/services/x/edit` still inside AppShell
- `/services/new/edit` → 404 (acceptable per spec)

- [ ] **Step 1:** move files, create layout + chrome components.
- [ ] **Step 2:** `bun run check` — fix type errors from the move (the moved pages' `$types` regenerate).
- [ ] **Step 3:** manual route check via `bun run dev` (or build if dev unavailable): the 4 routes above behave as specified.
- [ ] **Step 4:** commit `feat(main): (marketplace) route group + header/footer chrome`.

---

### Task 3: `/services` marketplace list page rebuild

**Files:**
- Rewrite: `(marketplace)/services/+page.server.ts` (keep the load logic; change `serviceCategories` mapping to keep FULL `ServiceCategoryInfo` objects — the API now returns icon/accent/description)
- Rewrite: `(marketplace)/services/+page.svelte`
- Create: `src/lib/components/marketplace/ServiceCard.svelte`
- Create: `src/lib/components/marketplace/CategoryPills.svelte`
- Create: `src/lib/components/marketplace/MarketplaceHero.svelte`

**Interfaces:**
- Consumes: `data.services: ServiceListing[]`, `data.meta` (pagination), `data.filters`, `data.serviceCategories: ServiceCategoryInfo[]`, `data.countries`, `data.packagesEnabled`, `data.authed`, `data.loadError`, `data.submitted`, root `data.currencies`, `data.features`.
- Produces: `ServiceCard` props `{listing: ServiceListing, currencies: CurrencyInfo[], categories: ServiceCategoryInfo[], authed: boolean}`; `CategoryPills` props `{categories: ServiceCategoryInfo[], active: string, kind: string}`; `MarketplaceHero` props `{q: string}` (emits search).

**List page structure** (spec §3.2):
1. `<svelte:head>` — title/description/meta from i18n + canonical.
2. `MarketplaceHero` — headline (`services.hero_title`), sub-copy, large `SearchInput` + submit; right column: 4 value-prop items (icon + label) — labels i18n'd literals (`services.value_*`), icons Lucide (`ShieldCheck`, `BadgePercent`, `Sparkles`, `Headset`).
3. `CategoryPills` — `All` + API categories filtered to current `kind` (`applies_to` covers kind; 'both' always shown), each pill: Lucide icon (map `icon` name → component via a fixed whitelist record in the component — unknown icon → `Grid2x2` fallback) + label + accent chip via `accentClasses`. Click writes `?category=`. `More…` overflow → Bits UI DropdownMenu when >8.
4. Kind tabs — `UrlTabs` Services/Packages writing `?kind=` (only when `packagesEnabled`); sort `SelectField` (Latest/Top); existing filter bar controls kept (dest/city/verified/price_max/date_from) — restyle to the marketplace filter row but preserve the same URL params.
5. Results grid — `grid sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-5`; `ServiceCard` per item.
6. States: `loadError` → existing retry block; empty → `EmptyState` with "adjust filters" copy; `submitted` → existing success notice; `PaginationControls` at bottom.
7. Trust strip below grid — 3 static i18n'd items (`services.trust_*`), Lucide icons.
8. Provider CTA band — "Are you a service provider?" → `/services/new` (authed) or `/signup` (guest).

**ServiceCard** (real `<a>` to `/services/{slug}`, `group` hover):
- Image area: `featured_image_url` → `<img loading="lazy">`; absent → category-tinted tile (`accentClasses` bg) with category icon centered.
- Ribbons: top-left — `has_coupon` → accent `BadgePercent` ribbon (`services.coupon_badge` "Deal"); `is_verified` → verified check chip. Top-right — wishlist heart: authed → POST bookmark — CHECK the bookmarks API first (`grep -rn "bookmark" internal/server/server.go apps/main/src/lib/api/`); if no service-listing bookmark endpoint exists, render heart → `/login` redirect for guests and OMIT the heart for authed (do not fake the action — note in report).
- Body: provider line (org name + verified icon or author name), title (2-line clamp), excerpt (2-line), category pill, rating row (`rating_avg`/`review_count` → stars + `(n)`; absent → hide row, no fake rating), price line `From {priceLabel}` (uses `min_price` → falls back via helper), `price_per` suffix.
- Whole card is the link; inner interactive elements (wishlist) use `on:click|stopPropagation`... NO — Svelte5: `{@attach}` or nested `<button>` inside `<a>` is invalid HTML — render card as `<article>` with a stretched-link pattern (`<a class="absolute inset-0">` + `relative` siblings with `z-10` for the heart). Check codebase for existing stretched-link card precedent (`DirectoryCard.svelte` likely does this — mirror it).

**Skeletons:** page shows `LoadingSkeleton` grid while SSR pending? SSR is blocking — skeletons only needed for client-side filter transitions; SvelteKit SSR navigation shows browser loading. Keep it simple: `navigating` state → dimmed grid + aria-busy (check if codebase uses `$app/navigation` `navigating` — grep one example).

- [ ] **Step 1:** update `+page.server.ts` (full category objects; everything else preserved verbatim).
- [ ] **Step 2:** build components + rewrite page; all strings via `t()`; add new keys to en.json + fr.json.
- [ ] **Step 3:** `bun run check` + `bun run test` clean; manually verify filters/tabs/search write URL params and SSR reloads.
- [ ] **Step 4:** commit `feat(main): marketplace services listing redesign`.

---

### Task 4: `/services/[slug]` detail rebuild — content sections

**Files:**
- Modify: `(marketplace)/services/[slug]/+page.server.ts` (no change needed — ServiceListing type now carries tiers/coupons; verify sidecars flow)
- Rewrite: `(marketplace)/services/[slug]/+page.svelte` (booking modal extracted to component — see Task 5)
- Create: `src/lib/components/marketplace/` — `TierGrid.svelte`, `CouponCard.svelte`, `IncludedGrid.svelte`, `HowItWorks.svelte`, `ProviderCard.svelte`, `ServiceReviews.svelte`, `ServiceFaq.svelte`, `StickyBookBar.svelte`
- Modify: `lib/i18n/en.json`, `fr.json`

**Interfaces:**
- Consumes: `data.service: ServiceListing` (+ `tiers`, `coupons` fields), `data.reviews`, `data.reviewAggregate`, `data.recommendedContent`, `data.myOrgs`, `data.authed`, `data.currentUserId`, `data.loadError`.
- Produces: `TierGrid` props `{tiers: PackageTier[], currencies, onSelect: (t: PackageTier) => void}`; `CouponCard` props `{coupon: ListingCoupon, currencies, purchaseMode: string}`; `ProviderCard` props `{service: ServiceListing}`; `BookingDialog` (Task 5) props `{service, tiers, departures, currencies, authed, open}`.

**Page structure** (spec §3.3 — mockup 4/5 layout on our tokens):
1. Breadcrumb: Home / Services / {category label} / {title}.
2. Hero split: left — category badge (accent chip + icon), H1, excerpt (first ~160 chars of body or `field_values` summary if present — check what the current page uses for excerpt), rating stars + `(n reviews)` + `view_count` "supported" stat, trust chips (Verified Provider when `is_verified`, Secure Booking, Expert Support — i18n literals), CTA pair: primary `Get started`/`Book now` (scrolls to booking / opens BookingDialog by mode), secondary `Talk to an expert` (inquiry path — check current page's inquiry flow and reuse). Right — featured image card (or category-tinted placeholder) + fact grid: destination, city, `price_per`, `available_from→until` window, capacity (render only fields that exist).
3. `IncludedGrid` — package `items[]` (`included` items with check icon; `optional` with dashed style + "Optional" tag); for plain services hide or derive from `field_values.features` if present — check existing page for how items/features render and reuse.
4. `HowItWorks` — numbered steps; source: `field_values.steps`/`how_it_works` if the schema has them (grep the create form for field names) — if absent, hide section entirely (Rule 16 — no invented steps).
5. `TierGrid` (packages + tiers flag on + `tiers?.length`): card per tier — badge, name, `formatMoney(price_amount, price_currency)`, one-time/per label, features checklist (check icons), `Choose {name}` → opens BookingDialog preselected to that tier; `is_featured` card gets primary ring + "Most popular" ribbon.
   - No tiers → single price card (existing pattern from current page).
6. `CouponCard` per `coupons[]` (flag on): title, discount label (`discountLabel`), code in dashed chip + Copy button (`navigator.clipboard` + copied toast/check state — check codebase for existing copy helper), expiry `Valid until {date}`, terms link, and when `referral_url` present (external coupon) → `Go to provider` outbound (open new tab, `rel="noopener sponsored"`).
7. `ServiceReviews` — `review_aggregate` stars/count + featured review card (first of `reviews`) + link to full reviews view (check existing review UX on the current page — reuse).
8. `ServiceFaq` — FAQ accordion (Bits UI Accordion) sourced from `field_values.faqs` if the schema carries it (check create form/`wikiFieldValue` helpers); absent → hidden.
9. `ProviderCard` — org name/slug → `/org/{slug}` or author link, verification badge, listing count if cheaply available (else omit), `View provider` link.
10. `StickyBookBar` — bottom sticky bar on mobile + right-column sticky card on desktop: `From {priceLabel}` + rating + primary CTA (purchase_mode aware: `paid` → "Book now"; `external` → "Visit provider" outbound `booking_url`; `inquiry` → "Send inquiry").
11. Recommended/related services row — keep existing `recommendedContent` card block, restyled to ServiceCard-ish links (reuse ServiceCard if shape compatible, else simple link cards).

- [ ] **Step 1:** components + page rewrite; strings i18n'd.
- [ ] **Step 2:** `bun run check` + tests clean; verify SSR page renders with flags on/off (tier section hidden when `tiers_enabled` off — the API omits `tiers` anyway; defensive `service.tiers?.length` guard).
- [ ] **Step 3:** commit `feat(main): marketplace service detail — content sections`.

---

### Task 5: Booking dialog with coupon + tier + price preview

**Files:**
- Create: `src/lib/components/marketplace/BookingDialog.svelte`
- Modify: `(marketplace)/services/[slug]/+page.svelte` (wire dialog; remove inline form)
- Modify: `lib/i18n/en.json`, `fr.json`

**Interfaces:**
- Consumes: existing booking form logic from old `+page.svelte` (lines ~160-290 — departure autocomplete, party size, preferred dates, provider/email for paid, `bookingIdemKey` per-open + regenerate-on-success, `POST /services/{id}/bookings`, checkout redirect on `payment.checkout_url`, inquiry success state). PRESERVE all of it verbatim where possible.
- Produces: `BookingDialog` props `{service: ServiceListing, currencies: CurrencyInfo[], authed: boolean, initialTierId?: string, bind:open}` — internal state for tier/coupon/quote.

**New behavior on top of the moved form:**
1. **Tier picker** (paid + `service.tiers?.length` + `initialTierId` support): radio-style tier rows (name, badge, formatted price) above departure select; sets `selectedTierId` → body `tier_id`.
2. **Coupon field** (when `service_coupons.enabled` — check `features` prop or `service.coupons !== undefined`): input + `Apply` button → `GET /services/{id}/price-preview?tier_id=&coupon_code=` via `api.get`.
   - `coupon_valid:true` → green check + line items: Subtotal `base`, Discount `−discount`, Fee due now `fee`, Balance to provider `balance_due` (all `formatMoney`).
   - `coupon_valid:false` → inline error keyed by `error_code` → i18n map (`coupon_min_spend` → "Minimum spend not met", `coupon_cap_reached` → "This coupon is fully redeemed", `coupon_user_limit` → "You've already used this coupon", `coupon_first_time_only` → "First-time customers only", `coupon_not_applicable` → "Not valid for this listing", `invalid_coupon` → "Invalid coupon code").
   - Re-preview on tier change while a valid coupon is applied.
   - 400 `invalid_tier` → tier deselect + error line.
3. Submit body gains `tier_id` + `coupon_code` (only when applied/valid — invalid code state blocks submit? No: submit passes `coupon_code` and backend revalidates — surface `invalid_coupon` etc. from POST response into the coupon error line; existing error mapping pattern).
4. `Idempotency-Key` — unchanged (`bookingIdemKey || crypto.randomUUID()`); form already always sends it.
5. Guest CTA: `authed=false` → dialog shows login redirect (`/login?next=/services/{slug}`), not the form — preserve existing guest behavior.

- [ ] **Step 1:** extract existing form markup/logic into `BookingDialog.svelte` (Modal or Bits UI Dialog — check codebase: `ui/Modal.svelte` exists; match its API), preserving behavior.
- [ ] **Step 2:** add tier picker + coupon preview + totals breakdown.
- [ ] **Step 3:** `bun run check` + `bun run test`; verify error-code mapping.
- [ ] **Step 4:** commit `feat(main): booking dialog with tiers + coupon price preview`.

---

### Task 6: SEO, i18n audit, polish, tests

**Files:**
- Modify: `(marketplace)/services/[slug]/+page.svelte` (`<svelte:head>` + JSON-LD)
- Modify: `(marketplace)/services/+page.svelte` (head/meta)
- Create: `apps/main/src/lib/components/marketplace/ServiceCard.test.ts` if component tests exist in repo convention (vitest + @testing-library? — CHECK `bun run test` setup first; if only lib-level tests exist, write a `marketplace.test.ts`-level test for the new pure helpers instead and note the absence of a component-test harness)
- Modify: `en.json`, `fr.json` (final pass — no missing keys)

**Requirements:**
1. Detail `<svelte:head>`: `{title} — Services | {siteName}`, `og:title/description/image` (featured_image_url), canonical `/services/{slug}`, JSON-LD `Service` (name, provider.organization name, offers: `Offer`/`AggregateOffer` with priceCurrency + lowPrice from min_price, aggregateRating when review_count>0). Escape via `{@html jsonScript}` pattern used elsewhere (grep `application/ld+json` for precedent — likely blog/[slug]).
2. List `<svelte:head>`: marketplace title/description + `og` defaults.
3. A11y pass: cards real links, focus-visible rings, `prefers-reduced-motion` on any animation, accordion/tab keyboard semantics via Bits UI.
4. Dark-mode spot check on every new component — tokens only.
5. Responsive check 360→1440 (hero stacks, pills scroll, grid 1→2→3→4, sticky bar mobile).
6. Flag-off checks: `service_packages.enabled` off → no Packages tab/tiers; `service_coupons.enabled` off → no coupon UI (server omits fields anyway — UI guards on field presence).
7. `bun run check` + `bun run test` + `bun run build` (if feasible) all clean.

- [ ] Commit `feat(main): marketplace SEO + a11y polish`.

## Spec coverage checklist

| Spec §3.x | Task |
|---|---|
| 3.1 route group + header/footer | T2 |
| 3.2 list page (hero/pills/tabs/cards/states) | T3 |
| 3.3 detail sections + sticky CTA + provider | T4 |
| 3.3 booking modal coupon+tier+preview | T5 |
| 3.3 SEO/JSON-LD | T6 |
| 3.5 i18n/a11y/tokens | all + T6 |
| Types for new payload fields | T1 |

Out of scope (Phase C / authoring plan): `/org/[slug]/coupons` manage UI, form tier/coupon/image fields on new/edit, booking list coupon columns.
