# Deals Redesign — CostGoat-style index/detail + frontend & org deal management

**Date:** 2026-10-05
**Status:** Draft — awaiting user review
**Refs:** `.jeos/sections/55-coupons-external-affiliates.md`, costgoat.com/deals + costgoat.com/deals/1bench.dev (design references)

## Problem

`/deals` and `/deal/{provider}/{slug}` are thin card grids and a single detail card.
CostGoat's deal pages are richer: a scannable deal table with category filters, and a
long-form deal page (active offer, secondary offer, "why" sections, "other ways to
save", FAQ) that reads like a proper landing page. There is also no way to create or
edit a deal from the main app — staff must use the admin panel — and organizations
have no way to submit deals.

## Decisions (from user)

- **Staff create/edit** gated on `coupons.manage`; **organization deal submissions**
  gated on org membership (owner/admin/editor role) landing in a **moderation queue**
  (`status='pending'`).
- Rich editorial content is stored on the coupon as a **`details` JSONB** column,
  with generated fallbacks so deals without editorial content still render well.
- **Category lives on `affiliate_providers`** — the provider is the single source of
  category; no per-coupon overrides. Internal coupons (no provider) are
  uncategorized.
- **Editing flows from the frontend as inline edit** — staff toggles edit mode on
  the deal detail page (and the provider landing page) instead of navigating to the
  admin panel.
- **"Other ways to save on {provider}"** is a data-driven card list of the
  provider's *other live deals* — not editorial content (no `details.savings`).
- **Admin panel**: the `/coupons` page becomes **Deals**, surfaces the new fields
  (`details`, provider `category`, `pending` status) and gets approve/reject
  moderation actions.

## Backend

### Migration `000794_coupon_details_provider_category.up.sql`

```sql
ALTER TABLE affiliate_providers ADD COLUMN category text;
ALTER TABLE affiliate_providers ADD COLUMN organization_id uuid REFERENCES organizations(id);
CREATE UNIQUE INDEX idx_affiliate_providers_org
  ON affiliate_providers (organization_id) WHERE organization_id IS NOT NULL;
ALTER TABLE coupons ADD COLUMN details jsonb;
ALTER TABLE coupons DROP CONSTRAINT coupons_status_check;
ALTER TABLE coupons ADD CONSTRAINT coupons_status_check
  CHECK (status IN ('active', 'expired', 'suspended', 'pending'));
```

- `affiliate_providers.category` — free text, no CHECK (categories evolve without
  migrations).
- `affiliate_providers.organization_id` — links a provider to the org that owns it;
  one provider per org (partial unique index). Providers auto-created for
  org-submitted deals.
- `coupons.details` — nullable JSONB:

```json
{
  "summary":  "string — lede under the H1",
  "trial":    { "title": "...", "body": "...", "url": "..." },
  "sections": [{ "title": "...", "body": "..." }],
  "faq":      [{ "q": "...", "a": "..." }]
}
```

All keys optional; validated server-side as a JSON object ≤ 32 KB
(`400 validation_error` otherwise). Unknown keys preserved, ignored at render.

- `pending` status: org submissions pending staff review. **Public endpoints never
  expose `pending` coupons** — `GetCouponBySlug` query gains `c.status <> 'pending'`
  and the public `GET /coupons/:id` handler 404s pending rows (admin reads go
  through `GetCouponByID`/`/admin/coupons/:id`, unaffected). `ListActiveCoupons`
  already filters `status='active'`.

### sqlc (`internal/db/queries/coupons.sql`) → `make sqlc-gen`

- Provider queries (`Create`/`Update`/`Get*`/`List*`): + `category`,
  `organization_id`.
- `CreateCoupon`, `UpdateCoupon`: + `details` (`sqlc.narg(details)`).
- `GetCouponBySlug`, `GetCouponByID`, `ListCoupons`, `ListActiveCoupons`: select
  `c.details`, `ap.category AS provider_category`. `GetCouponBySlug` also gains the
  `status <> 'pending'` guard.
- New `ListActiveDealPartners :many` — slim active-provider rows (`id, name, slug,
  description, category, logo_media_id, base_url, is_featured, organization_id`).
- New `ListCouponsByOrganization :many` — a provider's coupons incl. pending/
  suspended (org's own submissions view), joined via `ap.organization_id`.

### Service/handlers (`internal/coupons`)

- `CreateProviderInput` / `UpdateProviderInput`: + `Category *string`,
  `OrganizationID` stays admin-managed (not settable via provider CRUD inputs).
- `CreateCouponInput` / `UpdateCouponInput`: + `Details json.RawMessage`
  (validated as above).
- `ListDealPartners` → `GET /api/v1/deals/partners` (public) — active providers;
  powers the deal form's provider select and the category datalist (Rule 16).
- Org submission (wired under the organizations route group in server.go, handlers
  live in `internal/coupons` and take `orgService` membership check via a small
  interface to avoid import cycles — same adapter pattern as
  `couponRenderAdapter`):
  - `POST /api/v1/organizations/{id}/deals` — RequireAuth; caller must be an org
    member with role owner/admin/editor. Resolves the org's provider (auto-creates
    one from org name/slug/website_url + `organization_id` on first submission);
    creates the coupon with `status='pending'`, `coupon_type='external'`,
    `created_by=caller`. Accepts the same fields as `CreateCouponInput` minus
    provider_id/status/is_verified.
  - `GET /api/v1/organizations/{id}/deals` — RequireAuth + same role gate; lists
    the org's coupons in all statuses (pending review visibility).
- Moderation: `GET /admin/coupons?status=pending` (works once the status exists);
  approve = `PATCH /admin/coupons/{id} {status:'active'}`, reject =
  `{status:'suspended'}` — no dedicated endpoints needed.
- Cache: provider mutations (`Create`/`Update`/`DeleteProvider`, and provider
  auto-creation) must flush `jc:coupon_list:*` + `jc:coupon_slug:*` since provider
  name/category now render inside coupon payloads — reuse the existing
  invalidation helper. No new Valkey key patterns.
- Register `GET /deals/partners` alongside `/deals/partners/{slug}` (distinct chi
  patterns — no conflict).

### JEOS updates

- `.jeos/sections/55-coupons-external-affiliates.md` — schema block, API routes
  (`GET /deals/partners`, org submission routes), `pending` status, Deal Pages
  subsection (new page structure + staff/org create/edit UX).
- `.jeos/conventions/api-routes-catalog.md` — new routes.
- `.jeos/sections/03-rbac.md` / permission catalog — document that org deal
  submission is gated by org member role (owner/admin/editor), not a new node.

## Frontend (`apps/main`)

### Types (`src/lib/types/index.ts`)

```ts
export interface CouponDetails {
  summary?: string;
  trial?: { title?: string; body?: string; url?: string };
  sections?: { title?: string; body?: string }[];
  faq?: { q?: string; a?: string }[];
}
```

- `Coupon`: + `details?: CouponDetails | null`, `provider_category?: string | null`.
- `DealPartnerProvider`: + `category?: string | null`.
- New `DealPartnerRef` — slim provider row from `GET /deals/partners`.
- New `MyOrgDeal` reuse of `Coupon` for `GET /organizations/{id}/deals`.

### `/deals` — index redesign

- Header: `ListPageBar` title + "Last updated {max(updated_at)}" + **New deal**
  button → `/deals/new`. Visible when `coupons.manage` OR (logged-in AND member of
  ≥1 org with owner/admin/editor role — the load fetches `GET /me/organizations`
  for auth'd users and exposes `canSubmit`).
- Category pills: `All` + distinct `provider_category` from loaded coupons;
  client-side filter synced to `?category=`; uncategorized deals under `All` only.
- Table on `sm+` (Provider | Category | Deal | Details | Claim →), stacked rows on
  mobile. Row → `dealPath(coupon)`; category pill; verified badge kept.
- Empty/load-error states unchanged.

### `/deal/{provider}/{slug}` — rich detail page

1. Breadcrumb (Deals › provider › title) + "Last updated {updated_at}" + staff
   **Edit** button (`coupons.manage`).
2. H1 + `details.summary` (fallback `description`).
3. **Active offer** card — provider name, description, mono `code` + copy when
   present, primary CTA → `/go`. Verified badge.
4. **Secondary offer** card from `details.trial`.
5. **"Why {provider}"** numbered list from `details.sections`; fallback: generated
   "How to redeem" steps (copy code / follow link / discount at checkout).
6. **"Other ways to save on {provider}"** — cards for the provider's *other* live
   coupons: `+page.server.ts` also fetches `GET /coupons?provider_id={id}` and
   filters out the current deal; section skipped when empty.
7. **FAQ** accordion from `details.faq` + `FAQPage` JSON-LD (existing `Offer`
   JSON-LD kept).
8. **Inline edit**: Edit swaps the page body for `<DealForm mode="edit">`;
   Save → `PATCH /admin/coupons/{id}` → `invalidateAll()`; Cancel → back to view.

### `/deal/{provider}` — partner landing

- Card grid unchanged; staff **Edit provider** button
  (`affiliate.providers.manage`) → inline edit (name, description, category,
  base_url, is_featured, is_active) → `PATCH /admin/affiliate-providers/{id}`.

### `/deals/new` — create page (staff + org modes)

- `+page.server.ts`: 307 → `/login` for guests; loads `GET /deals/partners`
  always and `GET /me/organizations` for non-staff. Mode:
  - `coupons.manage` → **staff mode**: provider select, full field set incl.
    status/is_verified; submit → `POST /admin/coupons` → `goto(dealPath(created))`.
  - else org member with owner/admin/editor → **org mode**: org picker (auto when
    one), provider locked to that org's provider (shown read-only), no status/
    verified fields; submit → `POST /organizations/{id}/deals` → success state
    "submitted for review" + the org's existing submissions listed with status
    chips (from `GET /organizations/{id}/deals`).
  - else → empty state "Deals are submitted by organizations" + link to create/join.
- Shared **`DealForm.svelte`** (`src/lib/components/deals/`) — create/edit/staff/
  org modes:
  - Staff: provider select (from `/deals/partners`); "＋ New provider" toggle when
    `affiliate.providers.manage` → mini-form (name, slug, category, base_url,
    description) → `POST /admin/affiliate-providers` → selects the new provider.
    Category input = `<datalist>` fed by distinct categories from the provider
    list (Rule 16).
  - Coupon fields: title, slug (auto-slugified from title until touched), code
    (optional — code-less deals), discount_type/value/currency, referral_url,
    terms_url, description, valid_from, expires_at, (staff: is_verified, status).
  - Details editor: summary textarea; repeatable add/remove row editors for
    `sections` and `faq`; `trial` field group. Omits `details` entirely when empty.

## Admin panel (`apps/admin`)

- Rename `/coupons` → `/deals` (move route dir, update `routes.ts` + sidebar
  label "Deals"; keep `/coupons` → `/deals` redirect or update all links —
  routes.ts is the nav source of truth).
- Deals page updates:
  - Status **select** field (active/expired/suspended/pending) replacing the
    `is_active` checkbox conflation; status filter gains `pending`.
  - `provider_category` column + `details` editing via `formSnippet` — JSON
    textarea with parse-before-submit validation (admin EntityForm has no
    repeatable-group editor; JSON is pragmatic for staff).
  - Custom `actions`: **Approve** (PATCH status=active) / **Reject** (PATCH
    status=suspended) shown on `status==='pending'` rows via `show()` predicates,
    alongside View/Edit/Delete; `entityController.reload()` after each.
  - View snippet gains `details`/category display.
- Affiliate providers page (`/affiliate`): add `category` field to the provider
  form + column.

## i18n

New keys in `en.json` + `fr.json` (main) and admin locales under `deals.*` /
`deal.*` / `coupon.*` (last_updated, new_deal, submit_deal, pending_review,
categories, section headings, FAQ, trial, approve/reject, save/cancel, toasts).

## Testing / verification

- `make sqlc-gen`; `go build ./...`; `go test ./internal/coupons`.
- `bun run check` in `apps/main` and `apps/admin`.
- Manual: create deal as staff via `/deals/new`; submit as org member → verify
  pending hidden publicly, appears in admin Deals queue → approve → verify table/
  filter/detail/FAQ render + inline edit round-trip.

## Out of scope

- Per-coupon category overrides (provider is the single source of category).
- Rejection feedback notes / org notifications on moderation decisions.
- Deal submission by individual non-org users.
