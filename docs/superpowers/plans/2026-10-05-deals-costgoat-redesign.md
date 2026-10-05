# Deals CostGoat-Style Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild `/deals` and `/deal/{provider}/{slug}` as CostGoat-style rich deal pages, add staff inline create/edit plus organization deal submissions with a `pending` moderation queue, provider categories, and update the admin panel's coupon page to "Deals".

**Architecture:** Backend: migration adds `affiliate_providers.category` + `organization_id`, `coupons.details` JSONB, and `pending` coupon status; sqlc queries regenerated; new public `GET /deals/partners` and org-scoped `POST/GET /organizations/{id}/deals`. Frontend main app: redesigned deals index (table + category pills + New deal), rich deal detail page driven by `details` with generated fallbacks, shared `DealForm` component powering `/deals/new` and inline edit. Admin app: `/coupons` → `/deals` rename, new fields, approve/reject moderation actions.

**Tech Stack:** Go 1.x + chi + sqlc/pgx + Valkey (backend); SvelteKit 2 + Svelte 5 runes + Tailwind v4 (apps/main, apps/admin); golang-migrate for migrations.

**Spec:** `docs/superpowers/specs/2026-10-05-deals-costgoat-redesign-design.md`

## Global Constraints

- **Rule 18:** use `bun`, `bunx`, `bun run` for ALL JS tooling. Never `npm`/`npx`/`pnpm`/`yarn`.
- **Rule 2:** never hand-edit `internal/db/gen/*` — edit `internal/db/queries/*.sql`, run `make sqlc-gen`.
- **Rule 16:** no hardcoded option lists in frontend code. Category suggestions come from `GET /deals/partners`; currencies from `data.currencies`.
- **Rule 17:** no AI attribution anywhere — plain commit messages, no "Generated with" trailers, no agent comments.
- **Rule 5:** API errors via `errpkg.WriteError(w, status, "snake_case_code", "Message")`.
- **JEOS-first:** update `.jeos/sections/55-coupons-external-affiliates.md` + `.jeos/conventions/api-routes-catalog.md` in the same change as the code (Task 5).
- JEOS reference comments: `// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md` on new Go handlers and Svelte pages.
- All work commits to `main` (JEOS rule 6).
- sqlc conventions in this repo: `sqlc.narg()` for nullable params; `gen.Queries` rows are converted via `rowToProvider` in `internal/coupons/store.go`.
- Frontend `t()` signature: `t('key', 'English fallback')` from `$lib/i18n`. Add keys to `apps/main/src/lib/i18n/en.json` and `fr.json`.
- Frontend API: server-side via `apiContext(event)` + `apiGet<T>(ctx, path)` / `apiList<T>(ctx, path)` from `$lib/server/api`; client-side mutations via `api.post/patch` from `$lib/api/client` (CSRF handled automatically).

---

### Task 1: Migration + sqlc schema/query updates

**Files:**
- Create: `migrations/000794_coupon_details_provider_category.up.sql`
- Create: `migrations/000794_coupon_details_provider_category.down.sql`
- Modify: `internal/db/queries/coupons.sql`

**Interfaces:**
- Produces (new columns): `affiliate_providers.category text`, `affiliate_providers.organization_id uuid`, `coupons.details jsonb`, coupon status `'pending'` allowed.
- Produces (new queries): `ListActiveDealPartners`, `ListCouponsByOrganization`, `GetAffiliateProviderByOrgID` — consumed by Tasks 3–4.

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

- [ ] **Step 3: Update `internal/db/queries/coupons.sql`**

Add `category` and `organization_id` to every affiliate provider column list (SELECT and RETURNING in `CreateAffiliateProvider`, `GetAffiliateProviderByID`, `GetAffiliateProviderBySlug`, `ListAffiliateProviders`, `UpdateAffiliateProvider`). Example for `CreateAffiliateProvider`:

```sql
-- name: CreateAffiliateProvider :one
INSERT INTO affiliate_providers (name, slug, description, provider_type, logo_media_id, base_url, tracking_param, tracking_value, commission_rate, is_active, is_featured, category, organization_id) VALUES (@name, @slug, sqlc.narg(description), @provider_type, sqlc.narg(logo_media_id), sqlc.narg(base_url), sqlc.narg(tracking_param), sqlc.narg(tracking_value), sqlc.narg(commission_rate), @is_active, @is_featured, sqlc.narg(category), sqlc.narg(organization_id))
RETURNING id, name, slug, description, provider_type, logo_media_id,
 base_url, tracking_param, tracking_value, commission_rate,
 is_active, is_featured, author_commission_pct, category, organization_id,
 created_at, updated_at;
```

`UpdateAffiliateProvider` gains `category = sqlc.narg(category)` in SET (and the expanded RETURNING). `organization_id` is not settable via update.

Coupon queries: add `c.details` to the SELECT lists of `GetCouponByID`, `GetCouponByCode`, `ListCoupons`, `ListActiveCoupons`; add `ap.category AS provider_category` to those plus `GetCouponBySlug`. Add `details` to `CreateCoupon`/`UpdateCoupon` (param `sqlc.narg(details)`, expanded RETURNING).

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

- [ ] **Step 4: Regenerate + build**

```bash
make sqlc-gen && go build ./internal/... 
```

Expected: compiles (new params are optional struct fields; `rowToProvider` in `internal/coupons/store.go` still compiles — it must be extended in Task 3 to carry `Category`/`OrganizationID`).

- [ ] **Step 5: Commit**

```bash
git add migrations/000794_coupon_details_provider_category.*.sql internal/db/queries/coupons.sql internal/db/gen
git commit -m "feat(coupons): provider category + org link, coupon details JSONB, pending status (§55)"
```

---

### Task 2: Export `MemberRole` on organizations.Service

**Files:**
- Modify: `internal/organizations/service.go` (after `IsMember`, ~line 250)

**Interfaces:**
- Produces: `func (s *Service) MemberRole(ctx context.Context, orgID, userID pgtype.UUID) (string, error)` — returns `""` + `pgx.ErrNoRows`-wrapped error for non-members; consumed by the coupons `orgResolver` in Task 4.

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

### Task 3: Coupons service layer — details, category, org submissions

**Files:**
- Modify: `internal/coupons/types.go`
- Modify: `internal/coupons/service.go`
- Modify: `internal/coupons/store.go`
- Modify: `internal/coupons/service_test.go`

**Interfaces:**
- Consumes: `gen.ListActiveDealPartnersRow`, `gen.ListCouponsByOrganizationRow`, `gen.GetAffiliateProviderByOrgID` (Task 1); `organizations.Service.MemberRole` shape (Task 2 — but consumed via an interface defined here, so no import needed).
- Produces for Task 4:
  - `CouponStatusPending CouponStatus = "pending"` (const)
  - `type DealPartnerRef struct { ID, Name, Slug string; Description, Category, BaseURL *string; LogoMediaID pgtype.UUID; IsFeatured bool; OrganizationID pgtype.UUID }` (JSON-tagged)
  - `func (s *Service) ListDealPartners(ctx) ([]DealPartnerRef, error)`
  - `type SubmitDealInput = CreateCouponInput` (alias — org submissions reuse it)
  - `func (s *Service) SubmitOrgDeal(ctx, orgID, userID pgtype.UUID, input SubmitDealInput) (gen.CreateCouponRow, error)` — auto-creates the org's provider, status forced `pending`
  - `func (s *Service) ListOrgDeals(ctx, orgID pgtype.UUID, page, pageSize int32) ([]gen.ListCouponsByOrganizationRow, error)`
  - `ErrOrgProviderSlug` not needed — slug collisions auto-suffix.
  - `validateCouponDetails(raw json.RawMessage) error` (unexported, tested)

- [ ] **Step 1: Write the failing tests** (append to `service_test.go`)

```go
func TestValidateCouponDetails(t *testing.T) {
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
		{"oversize rejected", json.RawMessage(`{"s":"` + string(make([]byte, 40*1024)) + `"}`), true},
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

(`import "encoding/json"` at top of test file.)

- [ ] **Step 2: Run tests — expect FAIL** (`validateCouponDetails` / `CouponStatusPending` undefined)

```bash
go test ./internal/coupons -run 'TestValidateCouponDetails|TestPendingConstant' -v
```

- [ ] **Step 3: Implement in `types.go` and `service.go`**

`types.go` — add the const and the partner ref:

```go
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

`service.go`:

```go
// maxCouponDetailsBytes caps the coupons.details JSONB payload (§55).
const maxCouponDetailsBytes = 32 * 1024

// validateCouponDetails accepts nil/empty and JSON objects up to
// maxCouponDetailsBytes; anything else is a validation error.
func validateCouponDetails(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	if len(raw) > maxCouponDetailsBytes {
		return fmt.Errorf("details exceeds %d bytes", maxCouponDetailsBytes)
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil || v == nil {
		return fmt.Errorf("details must be a JSON object")
	}
	return nil
}
```

Add `Details json.RawMessage `json:"details,omitempty"`` to `CreateCouponInput` (`UpdateCouponInput` is already `type UpdateCouponInput = CreateCouponInput` — the alias covers it). In `CreateCoupon`/`UpdateCoupon`: call `validateCouponDetails(input.Details)` → on failure return the sentinel `ErrInvalidDetails = errors.New("invalid deal details")` (declare in types.go) and map it in `handleServiceError` to `400 validation_error`. **sqlc emits `[]byte` for jsonb** (see `Stats []byte` in the generated code) — the gen params field will be `Details []byte`, so pass `Details: []byte(input.Details)`; nil input → nil.

Add `Category *string` to `CreateProviderInput`; wire `Category: input.Category` in `CreateProvider`/`UpdateProvider` params.

`rowToProvider` in `store.go`: add `Category: r.Category, OrganizationID: r.OrganizationID` to every case.

**Cache invalidation for provider mutations:** extract the SCAN-delete of `jc:coupon_list:*`/`jc:coupon_slug:*` from `invalidateCouponCaches` into `flushDealCaches(ctx)` (keep the per-id/slug deletes in `invalidateCouponCaches`, which calls the shared helper). Call `s.flushDealCaches(ctx)` at the end of `CreateProvider`, `UpdateProvider`, `DeleteProvider` — provider name/category render inside coupon payloads.

```go
// flushDealCaches SCAN-deletes the coupon list and slug caches — provider
// mutations change fields embedded in coupon payloads (§55). Best-effort.
func (s *Service) flushDealCaches(ctx context.Context) {
	if s.vk == nil {
		return
	}
	for _, pattern := range []string{
		valkey.MustKey("coupon_list:*"),
		valkey.MustKey("coupon_slug:*"),
	} {
		iter := s.vk.Scan(ctx, 0, pattern, 100).Iterator()
		for iter.Next(ctx) {
			if err := s.vk.Del(ctx, iter.Val()).Err(); err != nil {
				slog.Warn("coupons: cache flush failed", "key", iter.Val(), "error", err)
			}
		}
	}
}
```

(Match the existing `invalidateCouponCaches` SCAN idiom — read its tail before writing; reuse its exact client calls.)

**Org submission:**

```go
// SubmitDealInput is the payload for POST /organizations/{id}/deals — a
// subset of CreateCouponInput; provider/status/verified are server-controlled.
type SubmitDealInput = CreateCouponInput

// SubmitOrgDeal creates a pending coupon under the organization's own
// affiliate provider (auto-created from the org on first submission).
// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
func (s *Service) SubmitOrgDeal(ctx context.Context, org gen.Organization, userID pgtype.UUID, input SubmitDealInput) (gen.CreateCouponRow, error) {
	if err := validateCouponDetails(input.Details); err != nil {
		return gen.CreateCouponRow{}, ErrInvalidDetails
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
		Details: []byte(input.Details), CreatedBy: userID,
	})
	if err != nil {
		return gen.CreateCouponRow{}, err
	}
	s.invalidateCouponCaches(ctx, c.ID, c.Slug)
	return c, nil
}

// providerForOrg returns the org's affiliate provider, creating it from the
// org's public profile on first use. Provider slug collisions suffix -2/-3…
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
			continue // slug taken
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

(`rowToProvider` gains a `gen.GetAffiliateProviderByOrgIDRow` case — same field list as the others.)

Also update `CreateProvider`/`UpdateProvider`/`DeleteProvider` to call `s.flushDealCaches(ctx)` after a successful write, and `GetProvider`-by-slug resolution already exists for reuse.

- [ ] **Step 4: Run tests + build**

```bash
go test ./internal/coupons -v && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/coupons internal/organizations && git commit -m "feat(coupons): deal details validation, provider category, org deal submissions (§55)"
```

---

### Task 4: Handlers + routes (partners index, org submit/list, pending guard)

**Files:**
- Modify: `internal/coupons/handlers.go`
- Modify: `internal/server/server.go`

**Interfaces:**
- Consumes: `Service.ListDealPartners`, `Service.SubmitOrgDeal`, `Service.ListOrgDeals`, `CouponStatusPending`, `DealPartnerRef` (Task 3); `organizations.Service.MemberRole`, `.GetOrganization` (Task 2).
- Produces (public API):
  - `GET /api/v1/deals/partners` → `{data: DealPartnerRef[]}`
  - `POST /api/v1/organizations/{id}/deals` → `{data: coupon}` (201)
  - `GET /api/v1/organizations/{id}/deals` → `{data: coupon[], page, page_size}`

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

Add field `orgs OrgResolver` + `func (h *Handlers) SetOrgResolver(o OrgResolver) { h.orgs = o }`. Roles allowed to submit: `owner`, `admin`, `editor` (org `member` read-only). Missing resolver → 503 `feature_unavailable`.

- [ ] **Step 2: Handlers**

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
// requireOrgEditor resolves the org and verifies the caller holds an
// owner/admin/editor role. Returns (org, user, ok); writes the error response.
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

In `handleServiceError`, add `ErrInvalidDetails` → `400 validation_error` (match how `ErrRequired` is mapped).

**Pending guard on the public coupon-by-id route** — in `GetCoupon` (public `GET /coupons/{id}`), after `h.Service.GetCoupon(...)`, 404 when pending:

```go
if coupon.Status == string(CouponStatusPending) {
	errpkg.WriteError(w, http.StatusNotFound, "not_found", ErrCouponNotFound.Error())
	return
}
```

(`GetCouponBySlug` already excludes pending in SQL — Task 1.)

- [ ] **Step 3: Wire routes in `server.go`**

After `couponHandlers.SetLiteralResolver(literalsCatalog)` (line ~883):

```go
couponHandlers.SetOrgResolver(orgService) // §55 org deal submissions
```

In the coupons route block (after the `/deals/{slug}/go` line, ~5442):

```go
r.Get("/deals/partners", couponHandlers.ListDealPartners)
```

In the organizations route group (near line 4263, alongside `/organizations/{id}/members`):

```go
// Organization deal submissions (§55) — member-role gated inside the handler.
r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).
	Post("/organizations/{id}/deals", couponHandlers.SubmitOrgDeal)
r.With(auth.RequireAuth(kr, vkClient.Redis())).
	Get("/organizations/{id}/deals", couponHandlers.ListOrgDeals)
```

(`couponHandlers` is constructed at line ~882, before routes — ordering is fine.)

- [ ] **Step 4: Build + test**

```bash
go build ./... && go test ./internal/coupons ./internal/organizations
```

- [ ] **Step 5: Commit**

```bash
git add internal/coupons internal/server && git commit -m "feat(deals): GET /deals/partners + org deal submission/moderation routes (§55)"
```

---

### Task 5: JEOS spec updates

**Files:**
- Modify: `.jeos/sections/55-coupons-external-affiliates.md`
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/sections/03-rbac.md` (one line — org-role gate note)

- [ ] **Step 1: Section 55 edits**

- Schema block: add `category text` + `organization_id uuid REFERENCES organizations(id)` (+ partial unique index note) to `affiliate_providers`; add `details jsonb` to `coupons`; extend the status CHECK comment to include `'pending'`.
- API Routes block, add under the deal-pages group:

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

- Deal Pages subsection: document `details` JSONB shape `{summary, trial, sections, faq}`, `provider_category` on coupon payloads, the "Other ways to save" sibling-deals section, `pending` invisibility on public endpoints, staff inline edit + `/deals/new` (staff vs org modes), moderation via `GET /admin/coupons?status=pending` + PATCH.

- [ ] **Step 2: `api-routes-catalog.md`** — add the three routes under the coupons/deals section in the same format as existing entries.

- [ ] **Step 3: `03-rbac.md`** — one line noting org deal submission/list is gated by `organization_members.role` (owner/admin/editor), not an RBAC node.

- [ ] **Step 4: Commit** `docs: section 55 — deal details, categories, org submissions, partners index`

---

### Task 6: Frontend types + `/deals` index redesign

**Files:**
- Modify: `apps/main/src/lib/types/index.ts` (~line 1954 `Coupon`, ~1994 `DealPartnerProvider`)
- Modify: `apps/main/src/routes/(app)/deals/+page.server.ts`
- Modify: `apps/main/src/routes/(app)/deals/+page.svelte`
- Modify: `apps/main/src/lib/i18n/en.json`, `fr.json`

**Interfaces:**
- Produces: `CouponDetails`, `Coupon.details`, `Coupon.provider_category`, `DealPartnerProvider.category`, `DealPartnerRef`, `MyOrganization` (check whether `SessionUser`-adjacent org type already exists — grep `member_role` in types/index.ts first; reuse if present).
- `/deals` PageData: `{ coupons: Coupon[], canSubmit: boolean, loadError: boolean, currencies }` (currencies already provided — check what the existing load returns; `data.currencies` is used so it comes from a parent layout or is added).

- [ ] **Step 1: Types** (in `types/index.ts`)

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
/** GET /deals/partners item — slim provider for selects/filter pills. */
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

/** GET /me/organizations item (embedded org fields + member_role). */
export interface MyOrganization {
	id: string;
	name: string;
	slug: string;
	member_role: 'owner' | 'admin' | 'editor' | 'member';
	[key: string]: unknown;
}
```

- [ ] **Step 2: `/deals/+page.server.ts`** — keep existing coupon fetch; add `canSubmit` for auth'd users:

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

Replace the card grid + partner strip with:

- Header row under `ListPageBar`: "Last updated {date}" (`max(coupons[].updated_at)` → `toLocaleDateString()`) and, when `data.canSubmit`, a primary link-button `New deal` → `/deals/new`.
- Category pills: `['All', ...distinct provider_category]` from `data.coupons`; `$state` selectedCategory seeded from `page.url.searchParams.get('category')`; selecting sets `?category=` via `goto('?category=…', { replaceState: true, noScroll: true })` (or `history.replaceState`) and filters `filteredCoupons` derived.
- Desktop table (`hidden sm:block`): columns Provider / Category / Deal / Details / Claim →
  - Provider: `provider_name` (or `t('deals.internal','JapaConnect')` when null), linked whole-row
  - Category: small pill `provider_category` or `—`
  - Deal: `title` + `discountLabel(coupon)`
  - Details: `description` truncated (`line-clamp-1`)
  - Claim: `ExternalLink` icon + `t('coupons.visit','Claim')` — cell content links to `dealPath(coupon)` (not `/go` — keep users on-site; row click handler `onclick={() => goto(dealPath(coupon))}` with `cursor-pointer`, plus `role="link"`/keyboard Enter for a11y)
- Mobile (`sm:hidden`): keep stacked rows — same info, compact flex rows.
- Keep `BadgeCheck` verified chip inline in the Deal cell; keep `recordEvent`/`copyCode` helpers removed if unused (code copy now lives on detail page — keep `copyCode` only if a code chip stays; simplest: drop code/copy from the row, show title+discount).
- Keep `EmptyState`, `loadError`, head/JSON-LD blocks.

i18n keys (en + fr): `deals.last_updated` ("Last updated {date}"), `deals.new_deal` ("New deal"), `deals.categories.all` ("All"), `deals.table.provider`/`category`/`deal`/`details`/`claim`, `deals.internal` ("JapaConnect").

- [ ] **Step 4: `bun run check`** in `apps/main` — fix type errors; commit.

---

### Task 7: Shared `DealForm` component

**Files:**
- Create: `apps/main/src/lib/components/deals/DealForm.svelte`
- Create: `apps/main/src/lib/components/deals/dealForm.ts` (types + payload builder — pure, testable)
- Test: `apps/main/src/lib/components/deals/dealForm.test.ts` (vitest — see `contentTypeRegistry.test.ts` for conventions)

**Interfaces:**
- Consumes: `Coupon`, `CouponDetails`, `DealPartnerRef`, `dealPath`, `api` client, `slugify` (import from `$lib/studio/prompt` — it exports `slugify`; acceptable reuse for a client-side hint since the server re-slugifies).
- Produces for Tasks 8–10:

```ts
export interface DealFormProps {
	mode: 'create' | 'edit';
	audience: 'staff' | 'org';
	coupon?: Coupon;                 // edit mode seed
	providers?: DealPartnerRef[];    // staff: provider select options
	canManageProviders?: boolean;    // staff: show "+ New provider" toggle
	orgId?: string;                  // org audience: target org id
	onsaved?: (coupon: Coupon) => void;
	oncancel?: () => void;
}
```

`buildCouponPayload(v: DealFormValues, audience): Record<string, unknown>` — pure builder (details omitted when all fields empty; `valid_until` key for expires, matching `CreateCouponInput`).

- [ ] **Step 1: Write `dealForm.test.ts`** — payload builder tests: empty details omitted; populated details nested; `expires_at` → `valid_until` ISO; empty code → absent.

- [ ] **Step 2: `dealForm.ts`** — `DealFormValues` interface, `buildCouponPayload`, `emptyDetails()`, `detailsFromCoupon(c)`.

- [ ] **Step 3: `DealForm.svelte`** — sections:
  - Provider block (staff only): `SelectField`/native `<select>` bound to `provider_id` from `providers`; when `canManageProviders`, a "New provider" disclosure with name/slug/category(datalist from `providers.map(p=>p.category)` deduped)/base_url/description — on submit-first flows, create provider via `api.post('/admin/affiliate-providers', …)` before the coupon POST, then use its id. Org audience: locked display of the org provider (name from a `providerName` prop or first org deal's provider) — actually simplest: hide provider UI entirely in org mode (server resolves it).
  - Coupon fields: title (required), slug (input; `slugTouched` flag — auto-fill `slugify(title)` until user edits), code (optional), discount_type select (`percent|fixed|free_shipping` options via i18n — these are backend enums, acceptable as static options since they mirror `coupons_discount_type_check`), discount_value number, currency (`data.currencies` via prop or plain text input fallback), referral_url, terms_url, description textarea, valid_from/expires_at `datetime-local`, staff-only `is_verified` checkbox + status select (active/suspended — pending/expired not settable in the form).
  - Details editor: summary textarea; `sections` and `faq` repeaters — `{#each}` rows with title/body (or q/a) inputs + remove button + "Add section"/"Add question" buttons; trial group (title/body/url).
  - Submit/Cancel buttons; `submitting` state; error line on failure (`res.error.message`).
  - Submit: create-staff → `api.post('/admin/coupons', payload)`; edit → `api.patch(\`/admin/coupons/${coupon.id}\`, payload)`; create-org → `api.post(\`/organizations/${orgId}/deals\`, payload)`.

- [ ] **Step 4: `bunx vitest run src/lib/components/deals` + `bun run check`**; commit.

---

### Task 8: `/deals/new` create page (staff + org modes)

**Files:**
- Create: `apps/main/src/routes/(app)/deals/new/+page.server.ts`
- Create: `apps/main/src/routes/(app)/deals/new/+page.svelte`

- [ ] **Step 1: `+page.server.ts`** — `/ama/new` gating pattern:

```ts
// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
if (!event.locals.user) throw redirect(307, `/login?redirect=${encodeURIComponent('/deals/new')}`);
const isStaff = !!event.locals.user.permissions?.includes('coupons.manage');
const ctx = apiContext(event);
let providers: DealPartnerRef[] = [];
let orgs: MyOrganization[] = [];
try { providers = (await apiList<DealPartnerRef>(ctx, '/deals/partners?page_size=100')).items; } catch {}
if (!isStaff) {
	try {
		orgs = (await apiList<MyOrganization>(ctx, '/me/organizations?page_size=50')).items
			.filter((o) => ['owner', 'admin', 'editor'].includes(o.member_role));
	} catch { orgs = []; }
}
return { isStaff, providers, orgs, canManageProviders: !!event.locals.user.permissions?.includes('affiliate.providers.manage') };
```

- [ ] **Step 2: `+page.svelte`**
  - `isStaff` → `<DealForm mode="create" audience="staff" {providers} canManageProviders={data.canManageProviders} onsaved={(c) => goto(dealPath(c))} />`
  - else `orgs.length > 0` → org picker (select when >1, auto-selected single org otherwise) → `<DealForm mode="create" audience="org" orgId={selectedOrg.id} onsaved={…} />`; on save show pending-review panel and fetch `api.get(\`/organizations/${org.id}/deals\`)` listing existing submissions with status chips (`active`/`pending`/`suspended`/`expired`).
  - else → `EmptyState` "Deals are submitted by organizations" + link to the org creation page (find the route — grep `routes/(app)` for `org`; fallback `/organizations`).
  - i18n keys: `deals.new_title`, `deals.org_picker`, `deals.submitted_pending`, `deals.no_org`, `deals.your_submissions`, `deals.status.*`.

- [ ] **Step 3: `bun run check`**; commit.

---

### Task 9: `/deal/{provider}/{slug}` rich detail + inline edit

**Files:**
- Modify: `apps/main/src/routes/(app)/deal/[provider]/[slug]/+page.server.ts`
- Modify: `apps/main/src/routes/(app)/deal/[provider]/[slug]/+page.svelte`

- [ ] **Step 1: `+page.server.ts`** — after resolving the coupon, fetch siblings for "Other ways to save" when `provider_id` is set:

```ts
let related: Coupon[] = [];
if (coupon.provider_id) {
	try {
		related = (await apiList<Coupon>(ctx, `/coupons?provider_id=${coupon.provider_id}&page_size=12`)).items
			.filter((c) => c.id !== coupon.id);
	} catch { related = []; }
}
return { coupon, related, canManage: !!event.locals.user?.permissions?.includes('coupons.manage'), providers: canManage ? (await apiList<DealPartnerRef>(ctx, '/deals/partners?page_size=100')).items : [] };
```

(Fetch providers only for staff so inline edit can switch provider.)

- [ ] **Step 2: `+page.svelte`** — rebuild the body (keep `<svelte:head>` + Offer JSON-LD; add FAQPage JSON-LD when `details.faq?.length`):

```jsonc
// append to the jsonLd object graph (separate script tag or @graph):
{ "@type": "FAQPage", "mainEntity": faq.map(f => ({
    "@type": "Question", "name": f.q,
    "acceptedAnswer": { "@type": "Answer", "text": f.a } })) }
```

Layout (all Tailwind var(--color-*) conventions from the existing page):
1. Header block: breadcrumb `Deals › {provider_name}`; "Last updated {coupon.updated_at → toLocaleDateString}"; when `data.canManage`, an Edit button toggling `editing = true`.
2. `{#if editing}` → `<DealForm mode="edit" audience="staff" coupon={data.coupon} providers={data.providers} canManageProviders={…includes('affiliate.providers.manage')} onsaved={() => { editing = false; invalidateAll(); }} oncancel={() => (editing = false)} />` — wrap form submission invalidate: call `await invalidateAll()` then set editing false (import `invalidateAll` from `$app/navigation`).
3. `{:else}` view:
   - H1 `coupon.title` + `details.summary ?? coupon.description` lede paragraph.
   - **Active offer** card: provider name + verified badge; `code` block + copy (`copyCode`/`copied` already in file); discountLabel; CTA → existing `goUrl`.
   - **Trial card** when `details.trial?.url || details.trial?.body`.
   - **"Why {provider_name ?? 'this deal'}"** — `{#each details.sections}` numbered list; when absent render a generated "How to redeem" ol (3 steps: follow link/copy code → apply at checkout → verify discount) behind `t('deal.how_to_redeem', …)`.
   - **"Other ways to save on {provider_name}"** — `{#each data.related}` compact cards (title + discountLabel + chevron) linking to `dealPath(c)`; skip when empty.
   - **FAQ** — `{#each details.faq}` `<details>/<summary>` accordion.
   - Terms link + expiry line kept from current page.

- [ ] **Step 3: i18n keys** — `deal.last_updated`, `deal.active_offer`, `deal.trial_cta`, `deal.why`, `deal.how_to_redeem`, `deal.redeem_step_*`, `deal.other_ways`, `deal.faq`, `deal.edit`, `common.cancel`/`common.save` (reuse if present).

- [ ] **Step 4: `bun run check`**; commit.

---

### Task 10: `/deal/{provider}` partner page + provider inline edit

**Files:**
- Modify: `apps/main/src/routes/(app)/deal/[provider]/+page.server.ts`
- Modify: `apps/main/src/routes/(app)/deal/[provider]/+page.svelte`

- [ ] **Step 1:** `+page.server.ts` — add `canManageProviders: !!event.locals.user?.permissions?.includes('affiliate.providers.manage')` to the return.

- [ ] **Step 2:** `+page.svelte` —
  - Show `provider.category` as a pill next to the provider name.
  - When `canManageProviders`: **Edit provider** button toggling an inline form (name, slug, description, category — datalist options optional since this is the source, base_url, is_featured, is_active checkboxes) → `api.patch(\`/admin/affiliate-providers/${provider.id}\`, body)` → `invalidateAll()`; Cancel closes. Plain inline `<form>` block under the header — no shared component needed (single use).
  - Keep the existing coupons grid.

- [ ] **Step 3: `bun run check`**; commit. i18n: `deal.edit_provider`, `deal.category`, form labels.

---

### Task 11: Admin panel — `/coupons` → `/deals` + new fields + moderation

**Files:**
- Rename: `apps/admin/src/routes/(app)/coupons/+page.svelte` → `apps/admin/src/routes/(app)/deals/+page.svelte`
- Modify: `apps/admin/src/lib/data/routes.ts` (`'/coupons'` → `'/deals'` — both occurrences)
- Modify: `apps/admin/src/lib/components/layout/sidebar.svelte` (line ~211: `path: "/coupons"` → `"/deals"`, label `nav.deals` "Deals")
- Modify: `apps/admin/src/routes/(app)/affiliate/+page.svelte` (provider form + table: add `category`)
- Modify: `apps/admin/src/lib/locales/en.json`, `fr.json`
- Redirect: grep for other `'/coupons'` links in `apps/admin/src` and update; add `apps/admin/src/routes/(app)/coupons/+page.server.ts` with `redirect(308, '/deals')` if any hard links remain — otherwise delete the dir.

- [ ] **Step 1: Route rename** — `git mv` the directory; update `routes.ts` (both the `adminRoutes` list and the earlier `'/coupons'` occurrence — check context: line 20 is probably a permissions/map list, keep semantics) and sidebar label.

- [ ] **Step 2: Page updates** (former coupons page):
  - `title: t('deals.title', 'Deals')`, `createLabel: 'New deal'`; keep `createPermission: 'coupons.manage'`.
  - Extend `Coupon` type: `details?: Record<string, unknown> | null; provider_category?: string | null;`.
  - `statusFilterOptions` += `{ value: 'pending', label: t('deals.pending','Pending review') }`.
  - Replace the `is_active` checkbox field with a `status` select (options: active/suspended/pending/expired — expired kept read-mostly but allowed for corrections); create/update payloads send `status: body.status ?? 'active'` (drop the `is_active` derivation); `load` mapping drops `is_active`.
  - Add `details` field: `{ key: 'details', label: t('deals.details_json','Deal page content (JSON)'), type: 'textarea', placeholder: '{"summary":"…","sections":[…],"faq":[…],"trial":{…}}' }`. In `load` map: `details: c.details ? JSON.stringify(c.details, null, 2) : ''`. In create/update: parse non-empty with `JSON.parse` inside try/catch → `{ ok:false, error: 'details must be valid JSON object' }`; send parsed object as `details`.
  - Columns: add `provider_category` (label "Category") between code and discount; `status` column stays (pending rows are now visible).
  - Custom `actions` (replaces defaults — include View/Edit/Delete explicitly per AGENTS.md): `View` → `ctx.openView(row)`, `Edit` → `ctx.openEdit(row)`, `Approve` (show: `row.status === 'pending'`) → `couponsAdminApiExtended.update(row.id, { status: 'active', title: row.title })` — wait, `UpdateCouponAdmin` requires `title` in body; send `title: row.title` plus `status`. Then `ctx.reload()`. `Reject` (same show predicate; variant destructive) → PATCH `{ status: 'suspended', title: row.title }` → reload. `Delete` → `ctx.openDelete(row)`.
  - View snippet: add `provider_category` and a `details` `<pre>` block when present.

- [ ] **Step 3: `/affiliate` page** — add `category` to the provider `FieldDef[]` (`type: 'text'` with placeholder e.g. "Cloud, AI, SaaS, Dev…") and a `category` column. Read the file first to match its structure.

- [ ] **Step 4: locales** — `nav.deals` ("Deals"/"Offres"), `deals.*` keys used above.

- [ ] **Step 5: `bun run check`** in `apps/admin`; commit.

---

### Task 12: End-to-end verification

- [ ] `make sqlc-gen` clean (no diff after regen), `go build ./...`, `go vet ./internal/coupons ./internal/organizations`, `go test ./internal/coupons ./internal/organizations`
- [ ] `bun run check` in `apps/main` and `apps/admin`
- [ ] Migration smoke: `make migrate` (or the project's migrate command — check Makefile) up+down on a dev DB if available; otherwise `psql` syntax-check the .up.sql
- [ ] Manual pass (backend + main app running): create deal as staff via `/deals/new` → lands on `/deal/{provider}/{slug}`; `/deals` table + category pills filter; detail page sections render (details populated vs fallback); staff inline edit round-trips; org member submits → pending → not visible on public `/deals`/`/deal/*` → appears in admin `/deals` → Approve → visible; provider inline edit sets category → pill appears.
- [ ] Final commit with any fixes.
