# Services Marketplace — Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the backend half of the services/packages marketplace upgrade: dynamic service categories, org-owned listing coupons with real checkout redemption, package pricing tiers, and enriched listing payloads (featured image, ratings, coupon badge).

**Architecture:** Extend `internal/servicedir` (§98/§111) and `internal/coupons` (§55) — no new packages. servicedir consumes coupon data through a `CouponStore` adapter seam (same pattern as `ReviewFetcher`/`PaymentCreator`), wired in `internal/server`. All data access via sqlc in `internal/db/queries/{services,coupons}.sql` → `make sqlc-gen`. Schema is **de-tenanted** (migration 000668 dropped tenant_id) — new tables have NO tenant columns.

**Tech Stack:** Go 1.x, chi router, sqlc+pgx, PostgreSQL, golang-migrate (`migrations/NNNNNN_name.{up,down}.sql`, next number **000803**), Valkey, feature flags via `internal/features/registry.go`.

## Global Constraints

- **JEOS first** (`/.jeos/AGENTS.md`): section files are source of truth; new permission nodes → `conventions/permission-key-naming.md` + seed migration; new settings keys → `conventions/system-settings-catalog.md` + seed; new Valkey keys → `conventions/valkey-keys-catalog.md`; new routes → `conventions/api-routes-catalog.md`.
- **Rule 2:** never hand-edit `internal/db/gen/*` — edit `internal/db/queries/*.sql` then `make sqlc-gen`.
- **Rule 3:** configurable values via `system_settings` (read through `s.settings.GetStringOr/GetIntOr/GetBoolOr` with identical code defaults).
- **Rule 5:** errors via `errpkg.WriteError(w, status, "snake_case_code", "message")`.
- **Rule 9:** `Idempotency-Key` required for paid bookings (already enforced — keep it ahead of coupon validation so a rejected coupon writes nothing).
- **Rule 17:** no AI attribution anywhere in code/commits.
- Existing enums stay valid: `service_category` remains `text` validated against the new table (hardcoded `validCategories` map becomes the fallback when the categories table is empty/unreachable).
- Money is `bigint` minor units. `coupons.discount_value` is `numeric` — percent = 0–100, fixed = minor units.
- Feature flags: register `service_coupons.enabled` and `service_packages.tiers_enabled` in `internal/features/registry.go` alongside the existing entries at lines ~72–73; gate every new public surface on them.
- Commits to `main` branch only. Commit after every task.

## Interfaces consumed/produced (cross-task contract)

```go
// internal/servicedir/coupons.go (new) — adapter seam, wired in internal/server
type CouponStore interface {
    ListListingCoupons(ctx, metaID pgtype.UUID, activeOnly bool) ([]ListingCoupon, error)
    GetListingCouponByCode(ctx, metaID pgtype.UUID, code string) (ListingCoupon, error)
    SetListingCoupons(ctx, metaID pgtype.UUID, couponIDs []pgtype.UUID) error
    GetCouponOrgID(ctx, couponID pgtype.UUID) (pgtype.UUID, error)
    CountRedemptions(ctx, couponID pgtype.UUID) (int64, error)
    CountRedemptionsByUser(ctx, couponID, userID pgtype.UUID) (int64, error)
    CreateRedemption(ctx, p CreateRedemptionParams) error
}

// CreateRedemptionParams — servicedir-local params for the seam; the
// server-side adapter maps it onto gen.CreateCouponRedemptionParams.
type CreateRedemptionParams struct {
    CouponID, UserID, BookingID, ListingMetaID pgtype.UUID
    AmountSaved int64
    Currency    string
}

type ListingCoupon struct {
    ID, Code, Title, DiscountType, ReferralURL, TermsURL string
    DiscountValue float64   // percent 0-100 or minor units per DiscountType
    Currency      string
    ExpiresAt     string    // RFC3339, "" when NULL
    Status        string
    MaxRedemptions, PerUserLimit *int64
    MinSpend      *int64
    FirstTimeOnly bool
    AppliesTo     string   // service|package|both
    OrgID         string
    Position      int32
}

// internal/servicedir/service_tiers.go (new)
type PackageTier struct {
    ID, Name, Badge, Description string
    PriceAmount int64; PriceCurrency string
    Features []string
    Position int32; IsFeatured bool
}
type TierInput struct { Name, Badge, Description string; PriceAmount int64; Features []string; IsFeatured bool }

// Quote is the single pricing source used by CreateBooking + price-preview.
type Quote struct {
    Base int64; Discount int64; Total int64; Fee int64; Balance int64
    Currency string; TierID pgtype.UUID; CouponID pgtype.UUID; CouponCode string
}
func (s *Service) priceQuote(ctx, listing listingRow, bookerID pgtype.UUID, tierID, couponCode string) (Quote, error)
```

New API routes (added to `api-routes-catalog.md` in the JEOS task):
```
GET    /admin/service-categories            rbac: service.categories.manage
POST   /admin/service-categories            rbac: service.categories.manage
PATCH  /admin/service-categories/{slug}     rbac: service.categories.manage
DELETE /admin/service-categories/{slug}     rbac: service.categories.manage
GET    /organizations/{id}/coupons          org editor role
POST   /organizations/{id}/coupons          org editor role
PATCH  /organizations/{id}/coupons/{cid}    org editor role
DELETE /organizations/{id}/coupons/{cid}    org editor role
GET    /services/{id}/coupons               listing editor
PUT    /services/{id}/coupons               listing editor
GET    /services/{id}/price-preview         authed, rate-limited
```

---

### Task 1: Migration `000803_service_categories`

**Files:**
- Create: `migrations/000803_service_categories.up.sql`, `migrations/000803_service_categories.down.sql`
- Modify: `internal/features/registry.go` (add 2 flag entries near line 73)

- [ ] **Step 1: Write the migration**

```sql
-- 000803_service_categories.up.sql
-- Services Marketplace — dynamic service_categories taxonomy replacing the
-- hardcoded §98 enum, plus seeds for the new marketplace categories.
-- Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md §2.1
BEGIN;

CREATE TABLE IF NOT EXISTS service_categories (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         text NOT NULL UNIQUE,
    label        text NOT NULL,
    description  text,
    icon         text,          -- lucide icon name
    accent_color text,          -- one of the token-mapped palette names
    applies_to   text NOT NULL DEFAULT 'both'
        CHECK (applies_to IN ('service','package','both')),
    position     int  NOT NULL DEFAULT 0,
    is_active    bool NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

INSERT INTO service_categories (slug, label, description, icon, accent_color, applies_to, position) VALUES
    ('immigration_legal',  'Immigration & Legal',  'Visas, PR, citizenship and legal help',            'scale',        'violet', 'both',    10),
    ('study_abroad',       'Study Abroad',         'Admissions, scholarships and student visas',       'graduation-cap','green', 'package', 20),
    ('education',          'Education',            'Schools, courses and credential evaluation',       'book-open',    'green',  'service', 30),
    ('work_abroad',        'Work Abroad',          'Jobs, CV review and interview prep',               'briefcase',    'amber',  'both',    40),
    ('relocation',         'Relocation',           'Accommodation, airport pickup and settling in',    'home',         'blue',   'both',    50),
    ('travel',             'Travel',               'Flights, itineraries and travel documents',        'plane',        'sky',    'both',    60),
    ('finance',            'Finance',              'Banking, transfers and tax help',                  'landmark',     'emerald','both',    70),
    ('health',             'Health & Insurance',   'Health coverage and insurance plans',              'heart-pulse',  'rose',   'both',    80),
    ('family_sponsorship', 'Family Sponsorship',   'Sponsor loved ones to join you abroad',            'users',        'orange', 'package', 90),
    ('documents',          'Documents',            'Translation, attestation and notarization',        'file-text',    'slate',  'service', 100),
    ('community',          'Community',            'Diaspora community services',                      'users-round',  'indigo', 'service', 110),
    ('other',              'Other',                'Everything else',                                  'grid-2x2',     'gray',   'both',    999)
ON CONFLICT (slug) DO NOTHING;

-- RBAC node (permission-key-naming: <area>.<entity>.<action>)
INSERT INTO permissions (key, description) VALUES
    ('service.categories.manage', 'Create/edit/delete service marketplace categories (admin)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role', 'admin-group-role')
  AND p.key = 'service.categories.manage'
ON CONFLICT DO NOTHING;

COMMIT;
```

```sql
-- 000803_service_categories.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'service.categories.manage');
DELETE FROM permissions WHERE key = 'service.categories.manage';
DROP TABLE IF EXISTS service_categories;
COMMIT;
```

- [ ] **Step 2: Register the feature flags**

In `internal/features/registry.go`, after the `service_packages.enabled` entry (~line 73):

```go
	{Key: "service_coupons.enabled", Label: "Service Listing Coupons", Default: false},
	{Key: "service_packages.tiers_enabled", Label: "Service Package Tiers", Default: false},
```

- [ ] **Step 3: Apply + verify**

Run: `make migrate` (or `go run ./cmd/migrate/`), confirm `service_categories` has 13 rows.
Run: `go build ./internal/features/` — expected PASS.

- [ ] **Step 4: Commit** `git add migrations/000803_service_categories.* internal/features/registry.go && git commit -m "feat: service_categories taxonomy + marketplace feature flags"`

---

### Task 2: Category queries + service integration

**Files:**
- Modify: `internal/db/queries/services.sql` (append block at end)
- Modify: `internal/servicedir/store.go` (category store methods), `internal/servicedir/service.go` (categorySet cache + IsValidCategory swap + Categories())

**Interfaces:**
- Produces: `Store.listCategories(ctx, activeOnly bool) ([]gen.ServiceCategory, error)`, `Service.IsValidCategoryDB(ctx, slug string) bool`, `Service.categorySet(ctx) map[string]struct{}`

- [ ] **Step 1: Append queries to `internal/db/queries/services.sql`**

```sql
-- ---------------------------------------------------------------------
-- service_categories (dynamic taxonomy — marketplace upgrade)
-- ---------------------------------------------------------------------

-- name: ListServiceCategories :many
SELECT id, slug, label, description, icon, accent_color, applies_to,
       position, is_active, created_at, updated_at
FROM service_categories
ORDER BY position ASC, label ASC;

-- name: GetServiceCategoryBySlug :one
SELECT id, slug, label, description, icon, accent_color, applies_to,
       position, is_active, created_at, updated_at
FROM service_categories
WHERE slug = @slug;

-- name: CreateServiceCategory :one
INSERT INTO service_categories (slug, label, description, icon, accent_color,
    applies_to, position, is_active)
VALUES (@slug, @label, sqlc.narg(description), sqlc.narg(icon),
    sqlc.narg(accent_color), COALESCE(sqlc.narg(applies_to), 'both'),
    COALESCE(sqlc.narg(position), 0), COALESCE(sqlc.narg(is_active), true))
RETURNING *;

-- name: UpdateServiceCategory :one
UPDATE service_categories
SET label = @label,
    description = sqlc.narg(description),
    icon = sqlc.narg(icon),
    accent_color = sqlc.narg(accent_color),
    applies_to = COALESCE(sqlc.narg(applies_to), applies_to),
    position = COALESCE(sqlc.narg(position), position),
    is_active = COALESCE(sqlc.narg(is_active), is_active),
    updated_at = now()
WHERE slug = @slug
RETURNING *;

-- name: DeleteServiceCategory :exec
DELETE FROM service_categories WHERE slug = @slug;

-- name: CountServiceListingsByCategory :one
SELECT COUNT(*) FROM service_listing_meta WHERE service_category = @slug;
```

- [ ] **Step 2: `make sqlc-gen`** — confirm `gen.ServiceCategory` + query methods exist.

- [ ] **Step 3: Store methods in `store.go`**

```go
func (s *Store) listCategories(ctx context.Context) ([]gen.ServiceCategory, error) {
	return s.read.ListServiceCategories(ctx)
}

func (s *Store) getCategoryBySlug(ctx context.Context, slug string) (gen.ServiceCategory, error) {
	return s.read.GetServiceCategoryBySlug(ctx, slug)
}
```

- [ ] **Step 4: Write the failing unit test** — `internal/servicedir/categories_test.go`:

```go
func TestCategorySetFallsBackToStaticEnum(t *testing.T) {
	// nil store → the hardcoded §98 enum is the fallback
	s := &Service{}
	set := s.categorySet(context.Background())
	for _, slug := range []string{"immigration_legal", "relocation", "finance", "education", "health", "community", "other"} {
		if _, ok := set[slug]; !ok {
			t.Fatalf("expected %q in fallback category set", slug)
		}
	}
}
```

Run `go test ./internal/servicedir/ -run TestCategorySet -v` — expect FAIL (`categorySet` undefined).

- [ ] **Step 5: Implement in `service.go`**

```go
// categorySet resolves the valid service_category set. Primary source is
// the service_categories table (dynamic taxonomy); the hardcoded §98 enum
// is the fallback when the store is unwired or the table errors — callers
// must never see a hard failure for category validation.
func (s *Service) categorySet(ctx context.Context) map[string]struct{} {
	if s.store != nil {
		if rows, err := s.store.listCategories(ctx); err == nil && len(rows) > 0 {
			set := make(map[string]struct{}, len(rows))
			for _, r := range rows {
				if r.IsActive {
					set[r.Slug] = struct{}{}
				}
			}
			return set
		}
	}
	return validCategories
}
```

Replace the three `IsValidCategory(...)` call sites in `service.go` (create ~line 440, edit ~line 1280) with `s.categorySet(ctx)` membership checks. Keep `IsValidCategory` exported (it's part of the package API / used by tests).

- [ ] **Step 6: Extend `HandleCategories` payload** — in `service.go` `Categories()` keep returning `[]Option` but add fields to `Option` (`icon`, `accent_color`, `applies_to`, `description` — all `omitempty`), populated from the DB rows (fallback: value/label only from `categoryLabels`).

- [ ] **Step 7:** `go test ./internal/servicedir/` PASS → commit `feat: dynamic service categories with DB-backed validation`.

---

### Task 3: Admin category CRUD

**Files:**
- Modify: `internal/servicedir/service.go` (`AdminCreateCategory/AdminUpdateCategory/AdminDeleteCategory`), `internal/servicedir/handlers.go` (4 handlers)
- Modify: `internal/server/server.go` (route block near line 6945)

- [ ] **Step 1: Service methods** (slug immutable; delete soft/hard per spec):

```go
func (s *Service) AdminCreateCategory(ctx context.Context, in CategoryInput) (gen.ServiceCategory, error)
func (s *Service) AdminUpdateCategory(ctx context.Context, slug string, in CategoryInput) (gen.ServiceCategory, error)
func (s *Service) AdminDeleteCategory(ctx context.Context, slug string) error
//   count := CountServiceListingsByCategory(slug); count>0 → UPDATE is_active=false; else DELETE
```

`CategoryInput{Slug, Label, Description, Icon, AccentColor, AppliesTo string; Position *int32; IsActive *bool}` — validate: slug `^[a-z][a-z0-9_]{1,48}$`, label non-empty ≤80, applies_to ∈ enum, accent_color ∈ palette `violet|green|amber|blue|sky|emerald|rose|orange|slate|indigo|gray` (Reject unknown → `invalid_category`/`invalid_input`).

- [ ] **Step 2: Handlers** — `HandleAdminListCategories`, `HandleAdminCreateCategory` (201), `HandleAdminUpdateCategory`, `HandleAdminDeleteCategory` (204). Errors via `errpkg.WriteError`; slug conflict → `409 category_exists`; missing → `404 not_found`.

- [ ] **Step 3: Wire routes** in `server.go` beside the other `/admin/services` routes (~line 6945):

```go
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "service.categories.manage")).
    Get("/admin/service-categories", serviceDirHandlers.HandleAdminListCategories)
// same guard for POST; PATCH/DELETE under /{slug}
```

- [ ] **Step 4:** `go build ./... && go test ./internal/servicedir/` PASS → commit.

---

### Task 4: Migration `000804_listing_coupons_and_tiers`

**Files:** `migrations/000804_listing_coupons_and_tiers.{up,down}.sql`

- [ ] **Step 1: Write the migration**

```sql
BEGIN;

-- coupons: org ownership + redemption rule set (all nullable/defaulted → zero downtime)
ALTER TABLE coupons
    ADD COLUMN IF NOT EXISTS organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS max_redemptions int,
    ADD COLUMN IF NOT EXISTS per_user_limit int,
    ADD COLUMN IF NOT EXISTS min_spend bigint,          -- minor units
    ADD COLUMN IF NOT EXISTS first_time_only boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS applies_to text NOT NULL DEFAULT 'both'
        CHECK (applies_to IN ('service','package','both'));

CREATE INDEX IF NOT EXISTS idx_coupons_org
    ON coupons (organization_id, status) WHERE organization_id IS NOT NULL;

-- listing ↔ coupon attach (ordered, per-listing overrides)
CREATE TABLE IF NOT EXISTS service_listing_coupons (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    coupon_id       uuid NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    listing_meta_id uuid NOT NULL REFERENCES service_listing_meta(id) ON DELETE CASCADE,
    position        int  NOT NULL DEFAULT 0,
    overrides       jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (coupon_id, listing_meta_id)
);
CREATE INDEX IF NOT EXISTS idx_slc_listing ON service_listing_coupons (listing_meta_id);

-- authoritative redemption record (counts exclude dead bookings — see query)
CREATE TABLE IF NOT EXISTS coupon_redemptions (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    coupon_id          uuid NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    user_id            uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    service_booking_id uuid REFERENCES service_bookings(id) ON DELETE SET NULL,
    listing_meta_id    uuid NOT NULL REFERENCES service_listing_meta(id) ON DELETE CASCADE,
    amount_saved       bigint NOT NULL DEFAULT 0,
    currency           text,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_redemptions_coupon ON coupon_redemptions (coupon_id);
CREATE INDEX IF NOT EXISTS idx_redemptions_user   ON coupon_redemptions (coupon_id, user_id);

-- package pricing tiers
CREATE TABLE IF NOT EXISTS service_package_tiers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    listing_meta_id uuid NOT NULL REFERENCES service_listing_meta(id) ON DELETE CASCADE,
    name            text NOT NULL,
    badge           text,
    description     text,
    price_amount    bigint NOT NULL,
    price_currency  text,
    features        text[] NOT NULL DEFAULT '{}',
    position        int  NOT NULL DEFAULT 0,
    is_featured     bool NOT NULL DEFAULT false,
    is_active       bool NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tiers_meta ON service_package_tiers (listing_meta_id, position);

-- booking snapshots
ALTER TABLE service_bookings
    ADD COLUMN IF NOT EXISTS tier_id uuid REFERENCES service_package_tiers(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS coupon_id uuid REFERENCES coupons(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS coupon_code text,
    ADD COLUMN IF NOT EXISTS discount_amount bigint;

-- settings (code defaults identical)
INSERT INTO system_settings (key, value) VALUES
    ('service_packages.coupons.max_per_listing', to_jsonb(3)),
    ('service_packages.tiers.max_count',         to_jsonb(5))
ON CONFLICT (key) DO NOTHING;

COMMIT;
```

Down migration drops in reverse order. Apply + verify columns exist. Commit.

---

### Task 5: sqlc queries for coupons/links/redemptions/tiers

**Files:** `internal/db/queries/coupons.sql` (append), `internal/db/queries/services.sql` (append tiers + booking columns on `CreateServiceBooking`)

- [ ] **Step 1: coupons.sql additions**

```sql
-- name: GetCouponByIDForOrg :one
SELECT c.* FROM coupons c WHERE c.id = @id AND c.organization_id = @organization_id;

-- name: ListOrgCoupons :many
SELECT c.id, c.title, c.slug, c.code, c.discount_type, c.discount_value,
       c.currency, c.status, c.valid_from, c.expires_at, c.applies_to,
       c.max_redemptions, c.per_user_limit, c.min_spend, c.first_time_only,
       c.terms_url, c.referral_url, c.created_at,
       (SELECT COUNT(*) FROM coupon_redemptions r WHERE r.coupon_id = c.id) AS redemption_count,
       (SELECT COUNT(*) FROM service_listing_coupons lc WHERE lc.coupon_id = c.id) AS listing_count
FROM coupons c
WHERE c.organization_id = @organization_id
ORDER BY c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateOrgCoupon :one
UPDATE coupons SET
    title = @title, description = sqlc.narg(description),
    code = @code, discount_type = sqlc.narg(discount_type),
    discount_value = sqlc.narg(discount_value), currency = sqlc.narg(currency),
    terms_url = sqlc.narg(terms_url), referral_url = sqlc.narg(referral_url),
    status = @status, valid_from = sqlc.narg(valid_from), expires_at = sqlc.narg(expires_at),
    max_redemptions = sqlc.narg(max_redemptions), per_user_limit = sqlc.narg(per_user_limit),
    min_spend = sqlc.narg(min_spend), first_time_only = COALESCE(sqlc.narg(first_time_only), first_time_only),
    applies_to = COALESCE(sqlc.narg(applies_to), applies_to), updated_at = now()
WHERE id = @id AND organization_id = @organization_id
RETURNING *;

-- name: DeleteOrgCoupon :execrows
DELETE FROM coupons WHERE id = @id AND organization_id = @organization_id;

-- name: ListServiceListingCoupons :many
SELECT c.id, c.title, c.code, c.discount_type, c.discount_value, c.currency,
       c.terms_url, c.referral_url, c.status, c.expires_at, c.applies_to,
       c.max_redemptions, c.per_user_limit, c.min_spend, c.first_time_only,
       c.organization_id, lc.position, lc.overrides
FROM service_listing_coupons lc
JOIN coupons c ON c.id = lc.coupon_id
WHERE lc.listing_meta_id = @listing_meta_id
ORDER BY lc.position ASC;

-- name: ListActiveServiceListingCoupons :many
SELECT c.id, c.title, c.code, c.discount_type, c.discount_value, c.currency,
       c.terms_url, c.referral_url, c.status, c.expires_at, c.applies_to,
       c.max_redemptions, c.per_user_limit, c.min_spend, c.first_time_only,
       c.organization_id, lc.position, lc.overrides
FROM service_listing_coupons lc
JOIN coupons c ON c.id = lc.coupon_id
WHERE lc.listing_meta_id = @listing_meta_id
  AND c.status = 'active'
  AND (c.valid_from IS NULL OR c.valid_from <= now())
  AND (c.expires_at IS NULL OR c.expires_at > now())
ORDER BY lc.position ASC;

-- name: GetListingCouponByCode :one
SELECT c.id, c.title, c.code, c.discount_type, c.discount_value, c.currency,
       c.terms_url, c.referral_url, c.status, c.expires_at, c.applies_to,
       c.max_redemptions, c.per_user_limit, c.min_spend, c.first_time_only,
       c.organization_id, lc.position, lc.overrides
FROM service_listing_coupons lc
JOIN coupons c ON c.id = lc.coupon_id
WHERE lc.listing_meta_id = @listing_meta_id
  AND lower(c.code) = lower(@code);

-- name: DeleteServiceListingCoupons :exec
DELETE FROM service_listing_coupons WHERE listing_meta_id = @listing_meta_id;

-- name: CreateServiceListingCouponLink :one
INSERT INTO service_listing_coupons (coupon_id, listing_meta_id, position)
VALUES (@coupon_id, @listing_meta_id, COALESCE(sqlc.narg(position), 0))
RETURNING *;

-- name: CountListingCouponLinks :one
SELECT COUNT(*) FROM service_listing_coupons WHERE listing_meta_id = @listing_meta_id;

-- name: CountCouponRedemptions :one
SELECT COUNT(*) FROM coupon_redemptions r
JOIN service_bookings b ON b.id = r.service_booking_id
WHERE r.coupon_id = @coupon_id
  AND b.status NOT IN ('cancelled','declined','refunded');

-- name: CountCouponRedemptionsByUser :one  -- same join + AND r.user_id=@user_id

-- name: CreateCouponRedemption :one
INSERT INTO coupon_redemptions (coupon_id, user_id, service_booking_id, listing_meta_id, amount_saved, currency)
VALUES (@coupon_id, @user_id, @service_booking_id, @listing_meta_id, @amount_saved, sqlc.narg(currency))
RETURNING *;

-- name: ListingHasActiveCoupon :one
SELECT EXISTS(SELECT 1 FROM service_listing_coupons lc JOIN coupons c ON c.id=lc.coupon_id
              WHERE lc.listing_meta_id=@listing_meta_id AND c.status='active'
                AND (c.expires_at IS NULL OR c.expires_at>now())) AS has_coupon;
```

- [ ] **Step 2: services.sql additions**

```sql
-- name: CreateServicePackageTier :one
INSERT INTO service_package_tiers (listing_meta_id, name, badge, description,
    price_amount, price_currency, features, position, is_featured, is_active)
VALUES (@listing_meta_id, @name, sqlc.narg(badge), sqlc.narg(description),
    @price_amount, sqlc.narg(price_currency), sqlc.narg(features),
    COALESCE(sqlc.narg(position), 0), COALESCE(sqlc.narg(is_featured), false),
    COALESCE(sqlc.narg(is_active), true))
RETURNING *;

-- name: ListServicePackageTiers :many
SELECT * FROM service_package_tiers
WHERE listing_meta_id = @listing_meta_id AND is_active = true
ORDER BY position ASC;

-- name: ListAllServicePackageTiers :many
SELECT * FROM service_package_tiers
WHERE listing_meta_id = @listing_meta_id
ORDER BY position ASC;

-- name: GetServicePackageTier :one
SELECT * FROM service_package_tiers
WHERE id = @id AND listing_meta_id = @listing_meta_id;

-- name: DeleteServicePackageTiers :exec
DELETE FROM service_package_tiers WHERE listing_meta_id = @listing_meta_id;

-- name: DeleteServicePackageTiersExcept :exec
DELETE FROM service_package_tiers
WHERE listing_meta_id = @listing_meta_id
  AND id <> ALL(@keep_ids::uuid[]);
-- name: GetMinActiveTierPrice :one
SELECT MIN(price_amount) FROM service_package_tiers
WHERE listing_meta_id = @listing_meta_id AND is_active = true;

-- name: CountServiceBookingsByUserOrg :one
SELECT COUNT(*) FROM service_bookings
WHERE booker_user_id = @user_id AND org_id = @org_id
  AND status NOT IN ('cancelled','declined','refunded');
```

Also extend `CreateServiceBooking` INSERT with `tier_id, coupon_id, coupon_code, discount_amount` (`sqlc.narg` each).

- [ ] **Step 3:** `make sqlc-gen` → `go build ./internal/db/gen/` PASS → commit `feat: sqlc queries for listing coupons, redemptions, tiers`.

---

### Task 6: Org coupon manage API (`internal/coupons`)

**Files:**
- Create: `internal/coupons/org_coupons.go` (service: `CreateOrgCoupon/ListOrgCoupons/UpdateOrgCoupon/DeleteOrgCoupon` + `validateOrgCouponInput`)
- Modify: `internal/coupons/handlers.go` (4 handlers reusing `requireOrgEditor`), `internal/coupons/store.go` (thin wrappers), `internal/server/server.go` (routes next to `/organizations/{id}/deals` ~line 4240)

- [ ] **Step 1: Write failing unit test** — `internal/coupons/org_coupons_test.go`: `validateOrgCouponInput` cases — missing code, bad discount_type, percent >100 or ≤0, fixed ≤0, free_shipping rejected (`unsupported_discount` — free_shipping meaningless for services), expires < valid_from, code regex `^[A-Z0-9_-]{3,32}$` (uppercased server-side), negative caps rejected.

- [ ] **Step 2: Service methods.** `CreateOrgCoupon(ctx, orgID, userID, OrgCouponInput)` → `coupon_type='internal'`, `organization_id=orgID`, `status='active'`, `is_verified=false`; code uniqueness → existing `ErrCouponAlreadyExists` (global `UNIQUE(code)` post-de-tenant — check `grep UNIQUE` on coupons migration state; treat 23505 on code as `coupon_code_taken` 409). `UpdateOrgCoupon` enforces org ownership via `GetCouponByIDForOrg` (missing → `ErrCouponNotFound`). `DeleteOrgCoupon`: if `redemption_count>0` → set `status='suspended'` instead of delete (audit trail), else hard delete row.

- [ ] **Step 3: Handlers + routes.** Reuse `requireOrgEditor`; list returns `{data, page, page_size}` parity with `ListOrgDeals`; mutations return `{data}`; delete → 204.

```go
r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/organizations/{id}/coupons", couponHandlers.ListOrgCoupons)
r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/organizations/{id}/coupons", couponHandlers.CreateOrgCoupon)
r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/organizations/{id}/coupons/{cid}", couponHandlers.UpdateOrgCoupon)
r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/organizations/{id}/coupons/{cid}", couponHandlers.DeleteOrgCoupon)
```

- [ ] **Step 4:** tests + build PASS → commit `feat: org-owned listing coupon CRUD API`.

---

### Task 7: Listing coupon attach + public payload (`internal/servicedir`)

**Files:**
- Create: `internal/servicedir/coupons.go` (`CouponStore` interface, `ListingCoupon`, `SetCouponStore` on Service, `SetListingCoupons`, `listingCoupons` payload builder)
- Create: `internal/coupons/listing_links.go` + `internal/server/coupon_adapter.go` (`serviceCouponAdapter` implementing `servicedir.CouponStore` over coupons.Store — mirrors `servicedir_adapters.go`)
- Modify: `internal/servicedir/handlers.go` (`HandleGetCoupons`, `HandleSetCoupons`), `service.go` (detail/list payload fields), `internal/server/server.go` (wiring + routes)

- [ ] **Step 1: `CouponStore` interface + `ListingCoupon` DTO** (contract block above — copy verbatim).

- [ ] **Step 2: coupons package link methods** — `listing_links.go`: `GetCouponOrgID`, `ListListingCoupons(activeOnly)`, `GetListingCouponByCode`, `SetListingCoupons` (delete+insert in tx via `s.write.WithTx`? store has no tx seam — use sequential execs under a `pgx.Tx` via the database pool like `service_bookings.go` does if available; otherwise delete-then-insert is acceptable because attach is editor-path, non-financial), `CountRedemptions*`, `CreateRedemption`.

- [ ] **Step 3: servicedir service methods** — `SetListingCoupons(ctx, editorUser, metaID, couponIDs)`: caller permission already resolved by handler (same edit-authorization path as `HandleEdit` — reuse `canEditListing` equivalent); validate: all coupons exist, `organization_id == listing.org_id` (coupons can't cross orgs), `applies_to` covers `listing_kind`, count ≤ `service_packages.coupons.max_per_listing` (default 3). `ListingCouponsDTO(ctx, metaID, public bool)` → []ListingCoupon sorted by position; `has_coupon` on list rows via `ListingHasActiveCoupon` (batch: extend `ListServiceListings` with a lateral EXISTS — modify the sqlc query instead of N+1).

- [ ] **Step 4: Handlers + routes + wiring**

```go
r.With(auth.RequireAuth(...), rbac.RequirePermission(rbacResolver, "service.edit")).
    Get("/services/{id}/coupons", serviceDirHandlers.HandleGetCoupons)
r.With(auth.RequireAuth(...), rbac.RequirePermission(rbacResolver, "service.edit")).
    Put("/services/{id}/coupons", serviceDirHandlers.HandleSetCoupons)
```

(check the actual node used by `PATCH /services/{id}` at server.go ~6941 — reuse it.) Wire `serviceDirService.SetCouponStore(&serviceCouponAdapter{...})` in server.go next to the `SetReviewFetcher` call site.

- [ ] **Step 5: Public payloads** — `GET /services/:slug` gains `coupons: []ListingCoupon` (activeOnly, omit `code` when flag `service_coupons.enabled` off); each `ListingCoupon` JSON: `{id, title, code, discount_type, discount_value, currency, expires_at, terms_url, referral_url, applies_to}`. List rows gain `has_coupon bool`. Gate both on `service_coupons.enabled`.

- [ ] **Step 6:** build + tests PASS → commit `feat: listing coupon attach API + public coupon payloads`.

---

### Task 8: Package tiers

**Files:** `internal/servicedir/service.go` (+ new `service_tiers.go`), `store.go`, `handlers.go`, `service_bookings.go` type additions only if needed here (TierInput validate)

- [ ] **Step 1: Failing test** — `service_tiers_test.go`: `validatePackageTiers` — empty ok, name required, >`tiers.max_count` rejected, price_amount ≥0 required, ≤1 `is_featured` (extra featured → `invalid_tiers`), unknown currency → pass-through (nullable).

- [ ] **Step 2: Service** — `validatePackageTiers([]TierInput) ([]gen.CreateServicePackageTierParams skeleton, error)`; on create/edit of `listing_kind='package'`, `input.Tiers` replaces the set (Delete + Insert loop, position = array index) — same replace-set pattern as `service_package_items` in `store_packages.go`; gate on `service_packages.tiers_enabled` flag → tiers field silently ignored when off (consistent with `PackagesEnabled` gating style).

- [ ] **Step 3: Payload** — detail: `tiers []PackageTier` (active only). List rows: `min_price` (lateral `GetMinActiveTierPrice` added to `ListServiceListings` select; fallback `price_amount`).

- [ ] **Step 4:** tests + build PASS → commit `feat: service package pricing tiers`.

---

### Task 9: Booking redemption + price preview

**Files:** `internal/servicedir/service_bookings.go` (extend), new `internal/servicedir/pricing.go` (`applyDiscount`, `priceQuote`), `handlers_bookings.go` (`HandlePricePreview`), `service_bookings_test.go` (extend), `server.go` (route)

**Interfaces:**
- Consumes: `CouponStore` (Task 7), tier queries (Task 5), `CountServiceBookingsByUserOrg` (Task 5).
- Produces: `CreateBookingInput{+TierID, +CouponCode}`, `Quote`, errors below.

- [ ] **Step 1: Failing tests** — `pricing_test.go`:

```go
func TestApplyDiscount(t *testing.T) {
	cases := []struct{ name, dtype string; val float64; price, want int64 }{
		{"percent 10%", "percent", 10, 100_000, 90_000},
		{"percent 100%", "percent", 100, 100_000, 0},
		{"percent floor", "percent", 150, 100_000, 0},
		{"fixed", "fixed", 25_000, 100_000, 75_000},
		{"fixed over price", "fixed", 150_000, 100_000, 0},
	}
	// assert each
}
```

- [ ] **Step 2: `pricing.go`** — `applyDiscount(base int64, dtype string, dval float64) int64` per table above; `priceQuote(ctx, listing, bookerID, tierID, couponCode)`:

```go
base := listing.PriceAmount (or tier.PriceAmount when tierID set — tier must belong to meta, be active → else ErrInvalidTier)
if couponCode != "" {
    lc := s.coupons.GetListingCouponByCode(ctx, metaID, couponCode)   // not-found → ErrInvalidCoupon
    check status active + window → ErrInvalidCoupon
    check lc.OrgID == listing.OrgID → ErrInvalidCoupon
    check lc.AppliesTo covers listing.ListingKind → ErrCouponNotApplicable
    if lc.MinSpend != nil && base < *lc.MinSpend → ErrCouponMinSpend
    if lc.MaxRedemptions != nil && CountRedemptions >= max → ErrCouponCapReached
    if lc.PerUserLimit != nil && CountRedemptionsByUser >= lim → ErrCouponUserLimit
    if lc.FirstTimeOnly && CountServiceBookingsByUserOrg(booker, org) > 0 → ErrCouponFirstTime
    discount := base - applyDiscount(base, lc.DiscountType, lc.DiscountValue)
}
total := base - discount; fee := s.computeFee(ctx, total); balance := total - fee
```

New sentinels: `ErrInvalidCoupon, ErrCouponNotApplicable, ErrCouponMinSpend, ErrCouponCapReached, ErrCouponUserLimit, ErrCouponFirstTime, ErrInvalidTier` → 400 codes `invalid_coupon, coupon_not_applicable, coupon_min_spend, coupon_cap_reached, coupon_user_limit, coupon_first_time_only, invalid_tier`.

- [ ] **Step 3: CreateBooking integration** — in `CreateBooking`, replace the `total = *listing.PriceAmount` block with `quote, err := s.priceQuote(...)` (bookerID = booker); require `Idempotency-Key` BEFORE coupon resolution for paid path (Rule 9: nothing writes on reject). Stamp `params.TierID/CouponID/CouponCode/DiscountAmount`. After `s.store.CreateBooking` succeeds and a coupon was applied: `s.coupons.CreateRedemption(ctx, {coupon_id, user_id, service_booking_id: booking.ID, listing_meta_id, amount_saved: discount, currency})` — redemption insert failure logs-and-continues (booking is authoritative; reconcile counts tolerate it) OR wrap booking+redemption in one tx if the store already exposes a tx seam — check `store.CreateBooking`; prefer the tx if trivially available. Inquiries (`intent='inquiry'`): ignore `coupon_code`/`tier_id` fields silently? No — reject `invalid_intent` parity with existing strictness: coupon on inquiry → `ErrInvalidCoupon`.

- [ ] **Step 4: `GET /services/{id}/price-preview`** — authed + `mw.RateLimit(...,"moderate")`; loads listing by content id, calls `priceQuote` with `?tier_id=&coupon_code=`; response:

```json
{ "data": { "base": 249900, "discount": 25000, "total": 224900, "fee": 22490, "balance_due": 202410, "currency": "USD", "coupon_valid": true } }
```

On coupon error → `200` with `{ "coupon_valid": false, "error_code": "coupon_min_spend", "base": ..., "total": ..., "fee": ..., "balance_due": ... }` (base-tier numbers still returned so the UI shows the undiscounted quote).

- [ ] **Step 5:** `go test ./internal/servicedir/` + `go build ./...` PASS → commit `feat: coupon redemption + tier pricing on service bookings`.

---

### Task 10: Featured image + ratings on listing payloads

**Files:** `internal/db/queries/services.sql` (`GetServiceListingBySlug`, `GetServiceListingByContentID`, `ListServiceListings`, `ListServiceListingsByOrg`), `internal/servicedir/service.go` + `handlers.go` DTO mapping

- [ ] **Step 1:** Add to each listing SELECT:
  - `c.featured_image_id, fm.storage_bucket AS featured_storage_bucket, fm.storage_path AS featured_storage_path` + `LEFT JOIN media fm ON fm.id = c.featured_image_id AND fm.deleted_at IS NULL` (pattern from `contents.sql` lines 92–109).
  - list query: `LEFT JOIN review_aggregates ra ON ra.entity_type = 'service_listing' AND ra.entity_id = c.id` → `ra.review_count AS rating_count, ra.average_rating AS rating_avg` (verify column names via `grep GetReviewAggregate internal/db/gen/reviews.sql.go`).
  - DTO: `featured_image_url` via `media.MediaURL(*bucket, *path, media.ImageProxyOpts{})` (pattern `internal/story/service.go:759`), `rating_avg float64`, `review_count int32`.

- [ ] **Step 2:** `make sqlc-gen`, build, test → commit `feat: featured image + review aggregate on service listings`.

---

### Task 11: JEOS catalog updates + final verification

**Files:** `.jeos/conventions/{api-routes-catalog.md, permission-key-naming.md, system-settings-catalog.md}` (+ create `.jeos/sections/112-services-marketplace.md` if JEOS convention requires a section per feature — follow the naming/structure of `111-service-packages-bookings.md`; JEOS lives in this repo so update it here)

- [ ] **Step 1:** Append the 11 new routes to `api-routes-catalog.md`; `service.categories.manage` to `permission-key-naming.md`; `service_packages.coupons.max_per_listing`, `service_packages.tiers.max_count` to `system-settings-catalog.md`; add the two feature flags to `feature-flag-naming.md` if that catalog lists keys.
- [ ] **Step 2:** `make check` (build+vet+lint+test) — zero new findings; `gosec` clean on touched files.
- [ ] **Step 3:** Commit `docs: JEOS catalogs for services marketplace backend`.

## Spec coverage checklist (self-review)

| Spec § | Task |
|---|---|
| 2.1 categories | T1–T3 |
| 2.2 coupon link + rules | T4–T7 |
| 2.3 checkout redemption + preview | T9 |
| 2.4 tiers | T4/T5/T8/T9 |
| 2.5 featured image + rating + has_coupon | T7/T10 |
| 2.6 migrations | T1/T4/T5 |
| flags/settings/RBAC | T1/T4/T6 (org-role authz) |
| JEOS catalogs | T11 |

Known follow-ups for Plans 2 & 3 (NOT this plan): `(marketplace)` route group + page redesigns, org coupon manage UI, listing form tier/coupon/image fields, admin category UI.
