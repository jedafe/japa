# Deals CostGoat-Style Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild `/deals` and `/deal/{provider}/{slug}` as CostGoat-style rich deal pages, add staff inline create/edit plus organization deal submissions with a `pending` moderation queue, provider categories, and update the admin panel's coupon page to "Deals".

**Architecture:** Backend: migration adds `affiliate_providers.category` + `organization_id`, `coupons.details` JSONB (sqlc column override → `json.RawMessage`), and `pending` coupon status; sqlc queries regenerated; new public `GET /deals/partners` and org-scoped `POST/GET /organizations/{id}/deals`. Frontend main app: redesigned deals index (table + category pills + New deal), rich deal detail page driven by `details` with generated fallbacks, shared `DealForm` component powering `/deals/new` and inline edit. Admin app: `/coupons` → `/deals` rename, new fields, approve/reject moderation actions.

**Tech Stack:** Go + chi + sqlc/pgx + Valkey (backend); SvelteKit 2 + Svelte 5 runes + Tailwind v4 (apps/main, apps/admin); golang-migrate for migrations; vitest for frontend unit tests.

**Spec:** `docs/superpowers/specs/2026-10-05-deals-costgoat-redesign-design.md`

## Global Constraints

- **Rule 18:** use `bun`, `bunx`, `bun run` for ALL JS tooling. Never `npm`/`npx`/`pnpm`/`yarn`.
- **Rule 2:** never hand-edit `internal/db/gen/*` — edit `internal/db/queries/*.sql` (and `sqlc.yaml` overrides), run `make sqlc-gen`.
- **Rule 16:** no hardcoded option lists in frontend code. Category suggestions come from `GET /deals/partners`; currencies from `data.currencies`. Backend-enum `<select>` options (discount_type, status) mirror DB CHECK constraints and are acceptable as static options.
- **Rule 17:** no AI attribution anywhere — plain commit messages, no "Generated with" trailers, no agent comments.
- **Rule 5:** API errors via `errpkg.WriteError(w, status, "snake_case_code", "Message")`.
- JEOS reference comments: `// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md` on new Go handlers and Svelte pages.
- All work commits to `main` (JEOS rule 6).
- sqlc conventions in this repo: `sqlc.narg()` for nullable params; provider rows are converted to `gen.AffiliateProvider` via `rowToProvider` in `internal/coupons/store.go`; `emit_json_tags` is on; **jsonb columns generate `[]byte` unless overridden** — `[]byte` marshals to base64 in `encoding/json`, which is why `coupons.details` gets a `json.RawMessage` column override (the `FeedSourceView.Stats json.RawMessage` remap in `internal/coupons/feed.go` exists for the same reason).
- Frontend `t()` signature: `t('key', 'English fallback')` from `$lib/i18n`. Add keys to `apps/main/src/lib/i18n/en.json` and `fr.json`.
- Frontend API: server-side via `apiContext(event)` + `apiGet<T>(ctx, path)` / `apiList<T>(ctx, path)` (returns `{items, meta?}`; works on non-paginated `{data:[]}` responses) from `$lib/server/api`; client-side via `api.get/post/patch` from `$lib/api/client` (CSRF handled automatically).
- `affiliate_providers.slug` has a unique index (`affiliate_providers_slug_key`) — provider slug collisions must suffix `-2`, `-3`, …

---

### Task 1: JEOS spec updates (JEOS-first rule — before any code)

**Files:**
- Modify: `.jeos/sections/55-coupons-external-affiliates.md`
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/sections/03-rbac.md`

- [ ] **Step 1: Section 55 edits**

- Schema block: `affiliate_providers` gains `category text` and
  `organization_id uuid REFERENCES organizations(id)` (+ partial unique index —
  one provider per org); `coupons` gains `details jsonb` (shape
  `{summary, trial, sections, faq}`) and status CHECK gains `'pending'`.
- API Routes block — under the deal-pages group add:

```
GET    /api/v1/deals/partners                  — public index of active providers
                                                 (id, name, slug, category, base_url)
                                                 [public; powers /deals filters + deal form]

-- Organization deal submissions
POST   /api/v1/organizations/:id/deals         — submit a deal; lands status='pending'
                                                 [auth; org role owner|admin|editor]
GET    /api/v1/organizations/:id/deals         — the org's own submissions, all statuses
                                                 [auth; org role owner|admin|editor]
```

- Deal Pages subsection: document `details` JSONB shape + `provider_category` on
  coupon payloads, the "Other ways to save on {provider}" sibling-deals section,
  `pending` invisibility on public endpoints, staff inline edit + `/deals/new`
  (staff vs org modes), moderation via `GET /admin/coupons?status=pending` +
  `PATCH /admin/coupons/:id {status}`.

- [ ] **Step 2: `api-routes-catalog.md`** — add the three routes under the coupons/deals section in the existing entry format.

- [ ] **Step 3: `03-rbac.md`** — one line: org deal submission/list is gated by `organization_members.role` (owner/admin/editor), not a new RBAC node.

- [ ] **Step 4: Commit** `docs(§55): deal details JSONB, provider category, org submissions, partners index`

---

### Task 2: Migration + sqlc override + query updates

**Files:**
- Create: `migrations/000794_coupon_details_provider_category.up.sql`
- Create: `migrations/000794_coupon_details_provider_category.down.sql`
- Modify: `sqlc.yaml` (one column override)
- Modify: `internal/db/queries/coupons.sql`
- Regenerate: `internal/db/gen/*` via `make sqlc-gen`

**Interfaces:**
- Produces (new columns): `affiliate_providers.category text`, `affiliate_providers.organization_id uuid`, `coupons.details jsonb`, coupon status `'pending'` allowed.
- Produces (new gen types): `gen.ListActiveDealPartnersRow` (fields: `ID, Name, Slug string; Description, Category, BaseUrl *string; LogoMediaID, OrganizationID pgtype.UUID; IsFeatured bool`), `gen.ListCouponsByOrganizationRow`, `gen.GetAffiliateProviderByOrgIDRow` — consumed by Task 4.
- Produces (type): coupon rows' `Details` field is `json.RawMessage` (column override), so it marshals as raw JSON in API responses (`null` when absent), not base64.

- [ ] **Step 1: Write the up migration**

```sql
-- 000794_coupon_details_provider_category.up.sql
--
-- CostGoat-style deal pages (§55):
--   affiliate_providers.category        — provider category for /deals filters
--   affiliate_providers.organization_id — links a provider to the org that owns
--                                         it; org members submit deals as pending
--   coupons.details                     — JSONB editorial content for the deal page
--   coupons.status + 'pending'          — org submissions awaiting moderation
--
-- Ref: .jeos/sections/55-coupons-external-affiliates.md

BEGIN;

ALTER TABLE affiliate_providers ADD COLUMN category text;
ALTER TABLE affiliate_providers ADD COLUMN organization_id uuid REFERENCES organizations(id);

CREATE UNIQUE INDEX idx_affiliate_providers_org
    ON affiliate_providers (organization_id) WHERE organization_id IS NOT NULL;

ALTER TABLE coupons ADD COLUMN details jsonb;

ALTER TABLE coupons DROP CONSTRAINT coupons_status_check;
ALTER TABLE coupons ADD CONSTRAINT coupons_status_check
    CHECK (status IN ('active', 'expired', 'suspended', 'pending'));

COMMIT;
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000794_coupon_details_provider_category.down.sql

BEGIN;

DELETE FROM coupons WHERE status = 'pending';

ALTER TABLE coupons DROP CONSTRAINT coupons_status_check;
ALTER TABLE coupons ADD CONSTRAINT coupons_status_check
    CHECK (status IN ('active', 'expired', 'suspended'));

ALTER TABLE coupons DROP COLUMN details;

DROP INDEX IF EXISTS idx_affiliate_providers_org;
ALTER TABLE affiliate_providers DROP COLUMN organization_id;
ALTER TABLE affiliate_providers DROP COLUMN category;

COMMIT;
```

- [ ] **Step 3: Add the `coupons.details` column override to `sqlc.yaml`**

In `sql.gen.go.overrides` (after the pgvector overrides):

```yaml
          # coupons.details is editorial JSON for the deal page; RawMessage so
          # API responses embed the object instead of base64 (§55).
          - column: "coupons.details"
            go_type:
              type: "RawMessage"
              import: "encoding/json"
              pointer: false
```

(If `column: "coupons.details"` doesn't match, sqlc accepts `public.coupons.details` — try the qualified form.)

- [ ] **Step 4: Update `internal/db/queries/coupons.sql`**

Add `category` and `organization_id` to every affiliate provider column list (SELECT lists and RETURNING in `CreateAffiliateProvider`, `GetAffiliateProviderByID`, `GetAffiliateProviderBySlug`, `ListAffiliateProviders`, `UpdateAffiliateProvider`). `CreateAffiliateProvider` gains `sqlc.narg(category), sqlc.narg(organization_id)` params; `UpdateAffiliateProvider` gains `category = sqlc.narg(category)` in SET (`organization_id` is never updated). Example:

```sql
-- name: CreateAffiliateProvider :one
INSERT INTO affiliate_providers (name, slug, description, provider_type, logo_media_id, base_url, tracking_param, tracking_value, commission_rate, is_active, is_featured, category, organization_id) VALUES (@name, @slug, sqlc.narg(description), @provider_type, sqlc.narg(logo_media_id), sqlc.narg(base_url), sqlc.narg(tracking_param), sqlc.narg(tracking_value), sqlc.narg(commission_rate), @is_active, @is_featured, sqlc.narg(category), sqlc.narg(organization_id))
RETURNING id, name, slug, description, provider_type, logo_media_id,
 base_url, tracking_param, tracking_value, commission_rate,
 is_active, is_featured, author_commission_pct, category, organization_id,
 created_at, updated_at;
```

Coupon queries: add `c.details` to the SELECT lists of `GetCouponByID`, `GetCouponByCode`, `ListCoupons`, `ListActiveCoupons`, `GetCouponBySlug`; add `ap.category AS provider_category` to all five. `CreateCoupon`/`UpdateCoupon` gain the `details` column/param (`sqlc.narg(details)`) and `details` in RETURNING.

`GetCouponBySlug` gains the pending guard:

```sql
WHERE c.slug = @slug AND c.status <> 'pending';
```

Append three new queries:

```sql
-- name: ListActiveDealPartners :many
-- Slim provider rows for the public GET /deals/partners index (§55) — powers
-- the /deals filter pills and the deal form's provider select.
SELECT id, name, slug, description, category, logo_media_id, base_url,
 is_featured, organization_id
FROM affiliate_providers
WHERE is_active = true
ORDER BY name ASC;

-- name: GetAffiliateProviderByOrgID :one
SELECT id, name, slug, description, provider_type, logo_media_id,
 base_url, tracking_param, tracking_value, commission_rate,
 is_active, is_featured, author_commission_pct, category, organization_id,
 created_at, updated_at
FROM affiliate_providers
WHERE organization_id = @organization_id;

-- name: ListCouponsByOrganization :many
-- All of an org's coupons in every status (org's own submissions view).
SELECT c.id, c.provider_id, c.coupon_type, c.title, c.slug, c.description,
 c.code, c.discount_type, c.discount_value, c.currency, c.terms_url,
 c.referral_url, c.is_verified, c.status, c.valid_from, c.expires_at,
 c.total_clicks, c.total_copies, c.created_by, c.created_at, c.updated_at,
 c.details,
 ap.name AS provider_name, ap.slug AS provider_slug, ap.logo_media_id AS provider_logo_media_id,
 ap.category AS provider_category
FROM coupons c
JOIN affiliate_providers ap ON ap.id = c.provider_id
WHERE ap.organization_id = @organization_id
ORDER BY c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;
```

- [ ] **Step 5: Regenerate + build**

```bash
make sqlc-gen && go build ./internal/...
```

Expected: compiles. Verify `gen.AffiliateProvider` now has `Category *string` and `OrganizationID pgtype.UUID`, and coupon rows have `Details json.RawMessage` — if `Details` came out `[]byte`, the override didn't take; fix the column key before proceeding.

- [ ] **Step 6: Commit**

```bash
git add migrations/000794_coupon_details_provider_category.*.sql internal/db/queries/coupons.sql sqlc.yaml internal/db/gen
git commit -m "feat(coupons): provider category + org link, coupon details JSONB, pending status (§55)"
```

---

### Task 3: Export `MemberRole` on organizations.Service

**Files:**
- Modify: `internal/organizations/service.go` (after `IsMember`, ~line 250)

**Interfaces:**
- Produces: `func (s *Service) MemberRole(ctx context.Context, orgID, userID pgtype.UUID) (string, error)` — returns the org-membership role; non-members get an error wrapping `pgx.ErrNoRows`. Consumed by the coupons `OrgResolver` in Task 5.
- Existing: `organizations.Service.GetOrganization(ctx, id pgtype.UUID) (gen.Organization, error)` (line ~245) — also consumed by Task 5.

- [ ] **Step 1: Add the method**

```go
// MemberRole returns the caller's role in the organization
// ("owner"|"admin"|"editor"|"member"). Non-members get an error wrapping
// pgx.ErrNoRows — callers should map that to 403, not 404 (BUG-021).
// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
// (organization deal submissions gate on this role).
func (s *Service) MemberRole(ctx context.Context, orgID, userID pgtype.UUID) (string, error) {
	return s.store.GetMemberRole(ctx, orgID, userID)
}
```

- [ ] **Step 2: Build + commit**

```bash
go build ./internal/organizations && git add internal/organizations/service.go && git commit -m "feat(organizations): export MemberRole for org-gated features (§55)"
```

---

### Task 4: Coupons service layer — details, category, org submissions

**Files:**
- Modify: `internal/coupons/types.go`
- Modify: `internal/coupons/service.go`
- Modify: `internal/coupons/store.go`
- Modify: `internal/coupons/service_test.go`

**Interfaces:**
- Consumes: Task 2 gen types; `gen.Organization` (fields used: `ID pgtype.UUID`, `Name`, `Slug`, `Description *string`, `WebsiteUrl *string`, `LogoMediaID pgtype.UUID`).
- Produces for Task 5:
  - `CouponStatusPending CouponStatus = "pending"` (const, types.go)
  - `ErrInvalidDetails = errors.New("invalid deal details")` (sentinel, types.go)
  - `type DealPartnerRef` JSON DTO (below) — returned by `Service.ListDealPartners`
  - `type SubmitDealInput = CreateCouponInput` (alias — org submissions reuse it; `ProviderID`/`Status`/`IsVerified` from the body are ignored — server controls them)
  - `func (s *Service) SubmitOrgDeal(ctx, org gen.Organization, userID pgtype.UUID, input SubmitDealInput) (gen.CreateCouponRow, error)`
  - `func (s *Service) ListOrgDeals(ctx, orgID pgtype.UUID, page, pageSize int32) ([]gen.ListCouponsByOrganizationRow, error)`
  - `func (s *Service) ListDealPartners(ctx context.Context) ([]DealPartnerRef, error)`
  - `validateCouponDetails(raw json.RawMessage) error` (unexported, unit-tested)

- [ ] **Step 1: Write the failing tests** (append to `service_test.go`; add `"encoding/json"` import)

```go
func TestValidateCouponDetails(t *testing.T) {
	oversize := json.RawMessage(`{"s":"` + string(make([]byte, 40*1024)) + `"}`)
	cases := []struct {
		name    string
		raw     json.RawMessage
		wantErr bool
	}{
		{"nil ok", nil, false},
		{"empty ok", json.RawMessage(""), false},
		{"object ok", json.RawMessage(`{"summary":"hi","faq":[{"q":"a","a":"b"}]}`), false},
		{"array rejected", json.RawMessage(`[1,2]`), true},
		{"scalar rejected", json.RawMessage(`"x"`), true},
		{"invalid json", json.RawMessage(`{bad`), true},
		{"oversize rejected", oversize, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCouponDetails(tc.raw)
			if tc.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestPendingConstant(t *testing.T) {
	if CouponStatusPending != "pending" {
		t.Error("wrong CouponStatusPending value")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL** (`validateCouponDetails` / `CouponStatusPending` undefined)

```bash
go test ./internal/coupons -run 'TestValidateCouponDetails|TestPendingConstant' -v
```

- [ ] **Step 3: Implement**

`types.go` — add:

```go
// ErrInvalidDetails is a sentinel error: coupons.details failed validation.
ErrInvalidDetails = errors.New("invalid deal details")

// CouponStatusPending marks organization-submitted deals awaiting staff
// review. Pending coupons are never exposed by public endpoints (§55).
CouponStatusPending CouponStatus = "pending"

// DealPartnerRef is the slim provider row returned by GET /deals/partners —
// powers the /deals filter pills and the deal form's provider select.
type DealPartnerRef struct {
	ID             pgtype.UUID `json:"id"`
	Name           string      `json:"name"`
	Slug           string      `json:"slug"`
	Description    *string     `json:"description,omitempty"`
	Category       *string     `json:"category,omitempty"`
	LogoMediaID    pgtype.UUID `json:"logo_media_id,omitempty"`
	BaseURL        *string     `json:"base_url,omitempty"`
	IsFeatured     bool        `json:"is_featured"`
	OrganizationID pgtype.UUID `json:"organization_id,omitempty"`
}
```

`service.go` — details validation + input field:

```go
// maxCouponDetailsBytes caps the coupons.details JSONB payload (§55).
const maxCouponDetailsBytes = 32 * 1024

// validateCouponDetails accepts nil/empty and JSON objects up to
// maxCouponDetailsBytes; anything else is ErrInvalidDetails.
func validateCouponDetails(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	if len(raw) > maxCouponDetailsBytes {
		return ErrInvalidDetails
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil || v == nil {
		return ErrInvalidDetails
	}
	return nil
}
```

Add `Details json.RawMessage `json:"details,omitempty"`` to `CreateCouponInput` (`UpdateCouponInput` is already `type UpdateCouponInput = CreateCouponInput`). In `CreateCoupon` and `UpdateCoupon`: `if err := validateCouponDetails(input.Details); err != nil { return <row>{}, err }` before the store call, and pass `Details: input.Details` (gen params emit `json.RawMessage` per the Task 2 override; if the param comes out `[]byte`, use `[]byte(input.Details)`).

Add `Category *string` to `CreateProviderInput`; wire `Category: input.Category` into `CreateProvider`/`UpdateProvider` params (gen field is `*string` — assign directly, no `toNullableUUID`-style helper needed).

`store.go` — `rowToProvider`: add `Category: r.Category, OrganizationID: r.OrganizationID` to **every** case, and add a `gen.GetAffiliateProviderByOrgIDRow` case with the same field list.

**Provider-mutation cache invalidation** — provider name/category render inside coupon payloads, so provider writes must flush list/slug caches. Reuse the existing `deleteKeysByPattern` helper (do NOT write a new Scan loop — `s.vk.Scan(ctx, cursor, pattern, n).Result()` is the codebase idiom). At the end of `CreateProvider`, `UpdateProvider`, `DeleteProvider` (after the successful write):

```go
// Provider fields render inside coupon payloads — flush list/slug caches.
s.deleteKeysByPattern(ctx, valkey.MustKey("coupon_list:*"))
s.deleteKeysByPattern(ctx, valkey.MustKey("coupon_slug:*"))
```

**Org submission + partners index** (append to service.go):

```go
// SubmitDealInput is the payload for POST /organizations/{id}/deals —
// provider_id/status/is_verified are server-controlled for org submissions.
type SubmitDealInput = CreateCouponInput

// SubmitOrgDeal creates a pending coupon under the organization's own
// affiliate provider (auto-created from the org on first submission).
// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
func (s *Service) SubmitOrgDeal(ctx context.Context, org gen.Organization, userID pgtype.UUID, input SubmitDealInput) (gen.CreateCouponRow, error) {
	if err := validateCouponDetails(input.Details); err != nil {
		return gen.CreateCouponRow{}, err
	}
	provider, err := s.providerForOrg(ctx, org)
	if err != nil {
		return gen.CreateCouponRow{}, err
	}
	slug, err := s.resolveCouponSlug(ctx, input.Slug, input.Title, pgtype.UUID{})
	if err != nil {
		return gen.CreateCouponRow{}, err
	}
	c, err := s.store.createCoupon(ctx, gen.CreateCouponParams{
		ProviderID: provider.ID, CouponType: string(CouponTypeExternal),
		Title: input.Title, Slug: slug, Description: input.Description,
		Code: strPtrOrNil(input.Code), DiscountType: input.DiscountType,
		DiscountValue: toNullableNumeric(input.DiscountValue), Currency: input.Currency,
		TermsUrl: input.TermsURL, ReferralUrl: input.ReferralURL,
		IsVerified: false, Status: string(CouponStatusPending),
		ValidFrom: toTimestamptz(input.ValidFrom), ExpiresAt: toTimestamptz(input.ExpiresAt),
		Details: input.Details, CreatedBy: userID,
	})
	if err != nil {
		return gen.CreateCouponRow{}, err
	}
	s.invalidateCouponCaches(ctx, c.ID, c.Slug)
	return c, nil
}

// providerForOrg returns the org's affiliate provider, creating it from the
// org's public profile on first use. affiliate_providers.slug is unique, so
// collisions suffix -2/-3… like resolveCouponSlug.
func (s *Service) providerForOrg(ctx context.Context, org gen.Organization) (gen.AffiliateProvider, error) {
	p, err := s.store.getProviderByOrgID(ctx, org.ID)
	if err == nil {
		return p, nil
	}
	if !isNoRows(err) {
		return gen.AffiliateProvider{}, fmt.Errorf("getting org provider: %w", err)
	}
	base := content.Slugify(org.Slug)
	if base == "" {
		base = content.Slugify(org.Name)
	}
	for i := 0; i < maxCouponSlugAttempts; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		if _, err := s.store.getProviderBySlug(ctx, candidate); err == nil {
			continue
		} else if !isNoRows(err) {
			return gen.AffiliateProvider{}, fmt.Errorf("checking provider slug: %w", err)
		}
		return s.store.createProvider(ctx, gen.CreateAffiliateProviderParams{
			Name: org.Name, Slug: candidate, Description: org.Description,
			ProviderType: string(ProviderTypeBoth), BaseUrl: org.WebsiteUrl,
			LogoMediaID: org.LogoMediaID, IsActive: true,
			OrganizationID: org.ID,
		})
	}
	return gen.AffiliateProvider{}, fmt.Errorf("could not find unique provider slug after %d attempts", maxCouponSlugAttempts)
}

// ListOrgDeals lists an org's coupons in all statuses (submissions view).
func (s *Service) ListOrgDeals(ctx context.Context, orgID pgtype.UUID, page, pageSize int32) ([]gen.ListCouponsByOrganizationRow, error) {
	limit, offset := paginate(page, pageSize)
	return s.store.listCouponsByOrganization(ctx, orgID, limit, offset)
}

// ListDealPartners returns active providers for GET /deals/partners.
func (s *Service) ListDealPartners(ctx context.Context) ([]DealPartnerRef, error) {
	rows, err := s.store.listActiveDealPartners(ctx)
	if err != nil {
		return nil, err
	}
	refs := make([]DealPartnerRef, 0, len(rows))
	for _, r := range rows {
		refs = append(refs, DealPartnerRef{
			ID: r.ID, Name: r.Name, Slug: r.Slug, Description: r.Description,
			Category: r.Category, LogoMediaID: r.LogoMediaID, BaseUrl: r.BaseUrl,
			IsFeatured: r.IsFeatured, OrganizationID: r.OrganizationID,
		})
	}
	return refs, nil
}
```

`store.go` additions:

```go
func (s *Store) getProviderByOrgID(ctx context.Context, orgID pgtype.UUID) (gen.AffiliateProvider, error) {
	row, err := s.read.GetAffiliateProviderByOrgID(ctx, orgID)
	if err != nil {
		return gen.AffiliateProvider{}, err
	}
	return rowToProvider(row), nil
}

func (s *Store) listActiveDealPartners(ctx context.Context) ([]gen.ListActiveDealPartnersRow, error) {
	return s.read.ListActiveDealPartners(ctx)
}

func (s *Store) listCouponsByOrganization(ctx context.Context, orgID pgtype.UUID, limit, offset int32) ([]gen.ListCouponsByOrganizationRow, error) {
	return s.read.ListCouponsByOrganization(ctx, gen.ListCouponsByOrganizationParams{
		OrganizationID: orgID, PageLimit: limit, PageOffset: offset,
	})
}
```

- [ ] **Step 4: Run tests + build**

```bash
go test ./internal/coupons -v && go build ./...
```

- [ ] **Step 5: Commit** `feat(coupons): deal details validation, provider category, org submissions (§55)`

---

### Task 5: Handlers + routes (partners index, org submit/list, pending guard)

**Files:**
- Modify: `internal/coupons/handlers.go`
- Modify: `internal/server/server.go`

**Interfaces:**
- Consumes: everything Task 4 produces; `organizations.Service.MemberRole`/`GetOrganization` (Task 3) via the `OrgResolver` interface below — wired in server.go, no import of `internal/organizations` needed in coupons.
- Produces (public API):
  - `GET /api/v1/deals/partners` → `{data: DealPartnerRef[]}`
  - `POST /api/v1/organizations/{id}/deals` → `{data: coupon}` (201)
  - `GET /api/v1/organizations/{id}/deals` → `{data: coupon[], page, page_size}`
  - `GET /api/v1/coupons/{id}` → 404 for `status='pending'`

- [ ] **Step 1: Org resolver interface + setter on `coupons.Handlers`**

```go
// OrgResolver is the slice of organizations.Service the deals handlers need —
// defined here to avoid an import cycle (same adapter pattern as
// couponRenderAdapter in server.go).
type OrgResolver interface {
	MemberRole(ctx context.Context, orgID, userID pgtype.UUID) (string, error)
	GetOrganization(ctx context.Context, id pgtype.UUID) (gen.Organization, error)
}
```

Add field `orgs OrgResolver` to `Handlers` + `func (h *Handlers) SetOrgResolver(o OrgResolver) { h.orgs = o }`.

- [ ] **Step 2: Handlers** (append to handlers.go)

```go
// ListDealPartners handles GET /api/v1/deals/partners — the public index of
// active deal providers (§55).
func (h *Handlers) ListDealPartners(w http.ResponseWriter, r *http.Request) {
	partners, err := h.Service.ListDealPartners(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	h.resolveLiterals(r, &partners)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": partners})
}
```

```go
// requireOrgEditor resolves the org from {id} and verifies the caller holds an
// owner/admin/editor role. Returns (org, user, ok); writes the error response
// on failure. Plain 'member' cannot submit deals.
func (h *Handlers) requireOrgEditor(w http.ResponseWriter, r *http.Request) (gen.Organization, *auth.AuthenticatedUser, bool) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return gen.Organization{}, nil, false
	}
	if h.orgs == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "feature_unavailable", "Organization deals are not available")
		return gen.Organization{}, nil, false
	}
	orgID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return gen.Organization{}, nil, false
	}
	org, err := h.orgs.GetOrganization(r.Context(), orgID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Organization not found")
		return gen.Organization{}, nil, false
	}
	role, err := h.orgs.MemberRole(r.Context(), orgID, user.UserID)
	if err != nil {
		// Non-member → 403 (BUG-021: never 404 for a permission denial).
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You must be an organization editor to manage deals")
		return gen.Organization{}, nil, false
	}
	if role != "owner" && role != "admin" && role != "editor" {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "Organization deal management requires an editor role")
		return gen.Organization{}, nil, false
	}
	return org, user, true
}
```

```go
// SubmitOrgDeal handles POST /api/v1/organizations/{id}/deals — org members
// submit deals that land as status='pending' for staff moderation (§55).
func (h *Handlers) SubmitOrgDeal(w http.ResponseWriter, r *http.Request) {
	org, user, ok := h.requireOrgEditor(w, r)
	if !ok {
		return
	}
	var input SubmitDealInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "title is required")
		return
	}
	coupon, err := h.Service.SubmitOrgDeal(r.Context(), org, user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": coupon})
}

// ListOrgDeals handles GET /api/v1/organizations/{id}/deals — the org's own
// submissions in every status (§55).
func (h *Handlers) ListOrgDeals(w http.ResponseWriter, r *http.Request) {
	org, _, ok := h.requireOrgEditor(w, r)
	if !ok {
		return
	}
	page := int32(parseQueryInt(r, "page", 1))
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize))
	coupons, err := h.Service.ListOrgDeals(r.Context(), org.ID, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	h.resolveLiterals(r, &coupons)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupons, "page": page, "page_size": pageSize})
}
```

Two small additions:
- `handleServiceError`: map `ErrInvalidDetails` → `errpkg.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())`.
- Pending guard on the public `GetCoupon` (`GET /coupons/{id}`): after `h.Service.GetCoupon(...)`, add `if coupon.Status == string(CouponStatusPending) { errpkg.WriteError(w, http.StatusNotFound, "not_found", ErrCouponNotFound.Error()); return }`. (Admin reads go through `GetCouponAdmin`, unaffected. `GetCouponBySlug` is already guarded in SQL.)

- [ ] **Step 3: Wire routes in `server.go`**

After `couponHandlers.SetLiteralResolver(literalsCatalog)` (~line 883):

```go
couponHandlers.SetOrgResolver(orgService) // §55 org deal submissions
```

In the deals route block (after the `/deals/{slug}/go` line, ~5442):

```go
r.Get("/deals/partners", couponHandlers.ListDealPartners)
```

In the organizations route group (~line 4263, alongside `/organizations/{id}/members`):

```go
// Organization deal submissions (§55) — org member-role gated inside the handler.
r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).
	Post("/organizations/{id}/deals", couponHandlers.SubmitOrgDeal)
r.With(auth.RequireAuth(kr, vkClient.Redis())).
	Get("/organizations/{id}/deals", couponHandlers.ListOrgDeals)
```

- [ ] **Step 4: Build + test**

```bash
go build ./... && go test ./internal/coupons ./internal/organizations
```

- [ ] **Step 5: Commit** `feat(deals): GET /deals/partners + org deal submission routes (§55)`

---

### Task 6: Frontend types + `/deals` index redesign

**Files:**
- Modify: `apps/main/src/lib/types/index.ts` (~line 1954 `Coupon`, ~1994 `DealPartnerProvider`)
- Modify: `apps/main/src/routes/(app)/deals/+page.server.ts`
- Modify: `apps/main/src/routes/(app)/deals/+page.svelte`
- Modify: `apps/main/src/lib/i18n/en.json`, `fr.json`

**Interfaces:**
- Produces (consumed by Tasks 7–9): `CouponDetails`, `Coupon.details`, `Coupon.provider_category`, `DealPartnerProvider.category`, `DealPartnerRef`, `MyOrganization`.
- `/deals` PageData gains `canSubmit: boolean`.

- [ ] **Step 1: Types** (`types/index.ts`)

```ts
/** coupons.details JSONB — editorial sections for the /deal page (§55). */
export interface CouponDetails {
	summary?: string;
	trial?: { title?: string; body?: string; url?: string };
	sections?: { title?: string; body?: string }[];
	faq?: { q?: string; a?: string }[];
}
```

Add to `Coupon`: `details?: CouponDetails | null;` and `provider_category?: string | null;`. Add to `DealPartnerProvider`: `category?: string | null;`. Add:

```ts
/** GET /deals/partners item — slim provider for selects/filter pills (§55). */
export interface DealPartnerRef {
	id: string;
	name: string;
	slug: string;
	description?: string | null;
	category?: string | null;
	logo_media_id?: string | null;
	base_url?: string | null;
	is_featured: boolean;
	organization_id?: string | null;
}

/** GET /me/organizations item — embedded org fields + caller's member_role. */
export interface MyOrganization {
	id: string;
	name: string;
	slug: string;
	member_role: 'owner' | 'admin' | 'editor' | 'member';
	[key: string]: unknown;
}
```

(First `grep "member_role" apps/main/src/lib/types/index.ts` — if an org-membership type already exists, reuse it instead of adding `MyOrganization`.)

- [ ] **Step 2: `/deals/+page.server.ts`** — keep the coupon fetch; add `canSubmit`:

```ts
let canSubmit = !!event.locals.user?.permissions?.includes('coupons.manage');
if (!canSubmit && event.locals.user) {
	try {
		const orgs = await apiList<MyOrganization>(ctx, '/me/organizations?page_size=50');
		canSubmit = orgs.items.some((o) => ['owner', 'admin', 'editor'].includes(o.member_role));
	} catch {
		canSubmit = false;
	}
}
return { coupons, pageSize: PAGE_SIZE, loadError, canSubmit };
```

- [ ] **Step 3: `/deals/+page.svelte` — costgoat layout**

- `ListPageBar` gets an `actions` snippet: "Last updated {date}" text (`max(coupons[].updated_at)` → `toLocaleDateString()`) + a primary link-button `New deal` → `/deals/new` rendered when `data.canSubmit`.
- Category pills under the bar: `['All', ...distinct provider_category]` derived from `data.coupons` (skip nulls). `selectedCategory` `$state` seeded from `page.url.searchParams.get('category')`; on select, `history.replaceState` the `?category=` param (no navigation) and `filteredCoupons` `$derived` filters. Uncategorized deals show only under "All". Replaces the partner-strip nav block entirely.
- Desktop (`hidden sm:block`): a real `<table>` — Provider / Category / Deal / Details / Claim columns. Whole row `onclick={() => goto(dealPath(coupon))}` + `onkeydown` Enter + `role="link"` + `cursor-pointer` (a11y). Cells: provider_name (or `t('deals.internal','JapaConnect')` when null); `provider_category` pill or `—`; `title` + `discountLabel` + `BadgeCheck` when verified; `description` with `line-clamp-1`; `ExternalLink` icon + `t('coupons.visit','Claim')`.
- Mobile (`sm:hidden`): compact stacked rows, same click-through.
- Drop the now-unused `copyCode`/`recordEvent`/`partners` helpers and unused imports (`Ticket`/`Store`/`Copy` if unused). Keep `EmptyState`, `loadError`, `<svelte:head>`/JSON-LD as-is.
- i18n (en + fr): `deals.last_updated` ("Last updated {date}"), `deals.new_deal` ("New deal" / "Nouvelle offre"), `deals.categories.all`, `deals.table.provider|category|deal|details|claim`, `deals.internal`.

- [ ] **Step 4: `bun run check`** in `apps/main`; commit `feat(main): costgoat-style /deals index — table, category pills, new-deal button (§55)`.

---

### Task 7: Shared `DealForm` component

**Files:**
- Create: `apps/main/src/lib/components/deals/dealForm.ts`
- Create: `apps/main/src/lib/components/deals/DealForm.svelte`
- Test: `apps/main/src/lib/components/deals/dealForm.test.ts` (vitest — follow `src/lib/components/create/contentTypeRegistry.test.ts` conventions)

**Interfaces:**
- Consumes: `Coupon`, `CouponDetails`, `DealPartnerRef`, `dealPath` (Task 6); `slugify` from `$lib/studio/prompt`; `api` from `$lib/api/client`; `t` from `$lib/i18n`.
- Produces for Tasks 8–10:

```ts
export interface DealFormProps {
	mode: 'create' | 'edit';
	audience: 'staff' | 'org';
	coupon?: Coupon;                  // edit-mode seed
	providers?: DealPartnerRef[];     // staff: provider select options
	canManageProviders?: boolean;     // staff: shows the "+ New provider" toggle
	orgId?: string;                   // org audience: target org id
	onsaved?: (coupon: Coupon) => void;
	oncancel?: () => void;
}
```

`buildCouponPayload(v: DealFormValues, audience: 'staff' | 'org'): Record<string, unknown>` — pure builder: omits `details` when all detail fields are empty; `expires_at` → `valid_until` (matches `CreateCouponInput`); empty `code`/`slug`/optional strings omitted; org audience omits `provider_id`/`status`/`is_verified`.

- [ ] **Step 1: Write `dealForm.test.ts`** — payload builder: details omitted when empty; populated details nested as `{summary, sections, faq, trial}`; `expires_at` datetime-local → `valid_until` ISO string; empty code omitted; org audience strips staff-only keys.

- [ ] **Step 2: `dealForm.ts`** — `DealFormValues` interface, `emptyDetails()`, `detailsFromCoupon(c: Coupon)` (maps `coupon.details` back into form values), `buildCouponPayload`. Run the test → green.

- [ ] **Step 3: `DealForm.svelte`** — Svelte 5 runes (`$props`, `$state`, `$derived`), Tailwind `var(--color-*)` conventions from the existing pages:

  - **Provider block** (staff audience only): `<select>` bound to `provider_id` listing `providers` (option label `name`, plus `category` suffix when set). When `canManageProviders`, a "+ New provider" toggle reveals name/slug/category/base_url/description inputs; category uses `<input list="deal-categories">` + `<datalist>` fed by deduped `providers.map(p => p.category)` (Rule 16 — not a hardcoded list). On submit, if the new-provider block is filled, POST `/admin/affiliate-providers` first and use the returned `id`.
  - **Coupon fields:** title (required), slug (auto-fills `slugify(title)` until the user edits it — `slugTouched` flag), code (optional), discount_type select (percent/fixed/free_shipping), discount_value number, currency (`data.currencies` prop threaded from page data — accept optional `currencies` prop, plain text input fallback), referral_url, terms_url, description textarea, valid_from/expires_at (`datetime-local`), staff-only `is_verified` checkbox + status select (active/suspended only — pending/expired are lifecycle states, not form options).
  - **Details editor:** summary textarea; `sections` repeater (title + body rows, remove button, "Add section"); `faq` repeater (q + a rows, "Add question"); trial group (title/body/url inputs).
  - **Submit:** `submitting` flag + error line from `res.error.message`. Create-staff → `api.post('/admin/coupons', payload)`; edit → `api.patch(`/admin/coupons/${coupon.id}`, payload)` (response row is `{data: coupon}` — unwrap `res.data.data` per the client's envelope shape: `api.patch<T>` returns `{data: T}` where T is the inner payload — check `ClientResult` and existing callers like `services/new` for the unwrap pattern); create-org → `api.post(`/organizations/${orgId}/deals`, payload)`. On success call `onsaved?.(createdCoupon)`.
  - i18n: namespace under `deal.form.*` (labels, add/remove buttons, new_provider, validation errors).

- [ ] **Step 4: `bunx vitest run src/lib/components/deals && bun run check`**; commit `feat(main): shared DealForm component for deal create/edit/submit (§55)`.

---

### Task 8: `/deals/new` create page (staff + org modes)

**Files:**
- Create: `apps/main/src/routes/(app)/deals/new/+page.server.ts`
- Create: `apps/main/src/routes/(app)/deals/new/+page.svelte`

- [ ] **Step 1: `+page.server.ts`** — the `/ama/new` gating pattern:

```ts
// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { apiContext, apiList } from '$lib/server/api';
import type { DealPartnerRef, MyOrganization } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent('/deals/new')}`);
	}
	const perms = event.locals.user.permissions ?? [];
	const isStaff = perms.includes('coupons.manage');
	const ctx = apiContext(event);
	let providers: DealPartnerRef[] = [];
	let orgs: MyOrganization[] = [];
	try {
		providers = (await apiList<DealPartnerRef>(ctx, '/deals/partners?page_size=100')).items;
	} catch {
		providers = [];
	}
	if (!isStaff) {
		try {
			orgs = (await apiList<MyOrganization>(ctx, '/me/organizations?page_size=50')).items.filter(
				(o) => ['owner', 'admin', 'editor'].includes(o.member_role)
			);
		} catch {
			orgs = [];
		}
	}
	return {
		isStaff,
		providers,
		orgs,
		canManageProviders: perms.includes('affiliate.providers.manage')
	};
};
```

(Verify `/deals/partners` actually accepts/ignores `page_size` — the handler returns the full list; `apiList` reads `data` regardless.)

- [ ] **Step 2: `+page.svelte`**

- `data.isStaff` → `<DealForm mode="create" audience="staff" providers={data.providers} canManageProviders={data.canManageProviders} onsaved={(c) => goto(dealPath(c))} />`
- else `data.orgs.length > 0` → org picker (`<select>` when >1 org, auto-select single) → `<DealForm mode="create" audience="org" orgId={selectedOrg.id} onsaved={…} />`; after save, show a "submitted for review" panel + fetch `api.get(`/organizations/${org.id}/deals`)` and render the org's submissions with status chips (reuse for the initial list too — load it when an org is selected).
- else → `EmptyState` "Deals are submitted by organizations" — find the org create route first (`grep -rn "organizations/new\|/org" apps/main/src/routes` — fallback `/organizations`).
- i18n: `deals.new_title`, `deals.org_picker`, `deals.submitted_pending`, `deals.no_org`, `deals.your_submissions`, `deals.status.pending|active|suspended|expired`.

- [ ] **Step 3: `bun run check`**; commit `feat(main): /deals/new — staff create + org submission modes (§55)`.

---

### Task 9: `/deal/{provider}/{slug}` rich detail + inline edit

**Files:**
- Modify: `apps/main/src/routes/(app)/deal/[provider]/[slug]/+page.server.ts`
- Modify: `apps/main/src/routes/(app)/deal/[provider]/[slug]/+page.svelte`

- [ ] **Step 1: `+page.server.ts`** — after the coupon resolves (keep the canonical redirect), add:

```ts
const canManage = !!event.locals.user?.permissions?.includes('coupons.manage');
let related: Coupon[] = [];
if (coupon.provider_id) {
	try {
		related = (await apiList<Coupon>(ctx, `/coupons?provider_id=${coupon.provider_id}&page_size=12`)).items.filter(
			(c) => c.id !== coupon.id
		);
	} catch { related = []; }
}
let providers: DealPartnerRef[] = [];
if (canManage) {
	try {
		providers = (await apiList<DealPartnerRef>(ctx, '/deals/partners')).items;
	} catch { providers = []; }
}
return { coupon, related, canManage, providers, canManageProviders: !!event.locals.user?.permissions?.includes('affiliate.providers.manage') };
```

- [ ] **Step 2: `+page.svelte`** — keep `<svelte:head>` + Offer JSON-LD; add a `FAQPage` JSON-LD object (via a second `safeJsonLdScript` call or `@graph`) when `coupon.details?.faq?.length`:

```jsonc
{ "@context": "https://schema.org", "@type": "FAQPage",
  "mainEntity": faq.map(f => ({ "@type": "Question", "name": f.q,
    "acceptedAnswer": { "@type": "Answer", "text": f.a } })) }
```

Body (reuse the existing color/radius vars):

1. Header: breadcrumb `All deals › {provider_name}`; "Last updated {coupon.updated_at → toLocaleDateString()}"; when `data.canManage`, an **Edit** button → `editing = true`.
2. `{#if editing}` → `<DealForm mode="edit" audience="staff" coupon={data.coupon} providers={data.providers} canManageProviders={data.canManageProviders} onsaved={async () => { await invalidateAll(); editing = false; }} oncancel={() => (editing = false)} />` (`import { invalidateAll } from '$app/navigation'`).
3. `{:else}` view, in order (each block skipped when its data is absent):
   - H1 `coupon.title` + lede (`details.summary ?? coupon.description`).
   - **Active offer** card: provider name + verified badge; `code` block + Copy (existing `copyCode`/`copied`); `discountLabel`; primary CTA → existing `goUrl`.
   - **Secondary offer** card from `details.trial` (title/body/CTA → `trial.url`, external + nofollow).
   - **"Why {provider_name ?? t('deal.this_deal','this deal')}"** — `{#each details.sections}` as an ordered list; when absent render a generated "How to redeem" `<ol>` (follow the deal link / copy the code if present / discount applies at checkout) — 3 i18n-backed steps.
   - **"Other ways to save on {provider_name}"** — `{#each data.related}` compact cards (title + discountLabel + chevron) linking `dealPath(c)`; skip when empty.
   - **FAQ** — `{#each details.faq}` `<details>/<summary>` accordion.
   - Keep terms link + expiry line.
4. i18n: `deal.last_updated`, `deal.active_offer`, `deal.trial_cta`, `deal.why`, `deal.how_to_redeem`, `deal.redeem_step_1..3`, `deal.other_ways`, `deal.faq`, `deal.edit` (en + fr).

- [ ] **Step 3: `bun run check`**; commit `feat(main): rich /deal detail page — summary, offers, sections, sibling deals, FAQ + inline edit (§55)`.

---

### Task 10: `/deal/{provider}` partner page + provider inline edit

**Files:**
- Modify: `apps/main/src/routes/(app)/deal/[provider]/+page.server.ts`
- Modify: `apps/main/src/routes/(app)/deal/[provider]/+page.svelte`

- [ ] **Step 1:** `+page.server.ts` — add `canManageProviders: !!event.locals.user?.permissions?.includes('affiliate.providers.manage')` to the return.

- [ ] **Step 2:** `+page.svelte` —
  - Show `provider.category` as a small pill next to the name (when set).
  - When `canManageProviders`: **Edit provider** button toggles `editing` → inline `<form>` under the header (name, slug, description textarea, category text, base_url url, is_featured + is_active checkboxes) → `api.patch(`/admin/affiliate-providers/${provider.id}`, { name, slug, description, category, base_url, is_featured, is_active, provider_type: provider.provider_type })` — `UpdateProvider` requires `name`; send the full field set — → `await invalidateAll()`; Cancel closes. Plain page-local form (single use, no shared component).
  - Keep the existing coupons grid.
- i18n: `deal.edit_provider`, `deal.category`, `deal.form.*` provider labels.

- [ ] **Step 3: `bun run check`**; commit `feat(main): /deal/{provider} category pill + staff provider inline edit (§55)`.

---

### Task 11: Admin panel — `/coupons` → `/deals` + new fields + moderation

**Files:**
- Rename: `apps/admin/src/routes/(app)/coupons/+page.svelte` → `apps/admin/src/routes/(app)/deals/+page.svelte` (`git mv`)
- Modify: `apps/admin/src/lib/data/routes.ts` — line ~99 `adminRoutes`: `'/coupons'` → `'/deals'`; line ~20 `mainRoutes`: `'/coupons'` → `'/deals'` (it lists main-site paths; the old path still 301s but keep it current)
- Modify: `apps/admin/src/lib/components/layout/sidebar.svelte` (line ~211: `path: "/coupons"` → `"/deals"`, label `t("nav.deals", "Deals")`)
- Modify: `apps/admin/src/routes/(app)/affiliate/+page.svelte` (provider form + columns: add `category`)
- Modify: `apps/admin/src/lib/locales/en.json`, `fr.json`
- `apps/admin/src/lib/api.ts` — **unchanged** (backend API stays `/admin/coupons`)
- Grep `apps/admin/src` for other `'/coupons'` links and update them to `/deals`.

- [ ] **Step 1: Route rename** — `git mv` the coupons dir to `deals`; update `routes.ts` + sidebar.

- [ ] **Step 2: Deals page updates** (the renamed file):
  - `title: t('deals.title', 'Deals')`, `createLabel: t('deals.create', 'New deal')`; `createPermission: 'coupons.manage'` unchanged.
  - Extend the local `Coupon` type: `details?: Record<string, unknown> | null; provider_category?: string | null;`.
  - `statusFilterOptions` += `{ value: 'pending', label: t('deals.pending', 'Pending review') }`.
  - Replace the `is_active` checkbox field with `{ key: 'status', label: t('common.status','Status'), type: 'select', options: [active, suspended, pending, expired] }`; drop the `is_active` derivation in `load` and send `status: body.status ?? 'active'` in create/update payloads.
  - Add field `{ key: 'details', label: t('deals.details_json', 'Deal page content (JSON)'), type: 'textarea' }` with a JSON-shape placeholder. In `load` map rows: `details: c.details ? JSON.stringify(c.details, null, 2) : ''`. In create/update: if `body.details` non-empty, `JSON.parse` it (try/catch → `{ ok:false, error: t('deals.details_invalid','details must be a JSON object') }`) and send the parsed object.
  - Columns: add `provider_category` ("Category") after `code`.
  - Custom `actions` array (replaces defaults — include View/Edit/Delete per AGENTS.md): `View` → `ctx.openView(row)`; `Edit` → `ctx.openEdit(row)`; `Approve` (`show: row => row.status === 'pending'`) and `Reject` (`variant: 'destructive'`) → PATCH via `couponsAdminApiExtended.update(row.id, payload)` then `ctx.reload()`. **IMPORTANT:** `UpdateCoupon` is a full-column overwrite — the moderation payload MUST be the complete field set reconstructed from `row` (reuse the same builder the update path uses: provider_id, coupon_type, title, slug, code, description, discount_type, discount_value, currency, terms_url, referral_url, is_verified, valid_from, valid_until, details re-parsed) with only `status` overridden ('active'/'suspended'). Sending `{title, status}` alone NULLs every other column. `Delete` → `ctx.openDelete(row)`.
  - View snippet: add `provider_category` line + `details` `<pre>` block when present.

- [ ] **Step 3: `/affiliate` page** — read it first; add `category` to the provider `FieldDef[]` (`type: 'text'`, placeholder e.g. "Cloud, AI, SaaS, Dev…") and a `category` column on the providers table.

- [ ] **Step 4: Locales** — `nav.deals`, `deals.*` keys above (en + fr).

- [ ] **Step 5: `bun run check`** in `apps/admin`; commit `feat(admin): rename coupons→deals, details/category fields, pending moderation actions (§55)`.

---

### Task 12: End-to-end verification

- [ ] `make sqlc-gen` produces no diff; `go build ./...`; `go vet ./internal/coupons ./internal/organizations`; `go test ./internal/coupons ./internal/organizations`
- [ ] `bun run check` clean in `apps/main` and `apps/admin`
- [ ] Migration smoke: `make migrate` up + down on the dev DB if running (check Makefile target name), else `psql` dry-run the up file
- [ ] Manual pass (backend + main app running): staff creates a deal via `/deals/new` → lands on `/deal/{provider}/{slug}`; `/deals` table + category pills; detail page sections render (populated details vs fallbacks); staff inline edit round-trips; org member submits → pending → invisible on public pages → appears in admin `/deals` queue → Approve → visible; provider inline edit sets category → pill shows on `/deals`.
- [ ] Final commit with any fixes.
