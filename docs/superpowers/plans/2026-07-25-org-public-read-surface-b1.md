# Org Public Read Surface (Phase B, Sub-project B1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the organization public profile (`/org/[slug]`) server-able by guests — a lean public header, a public Members tab, and a public verified org list — reusing the Phase A public-profile patterns, with NO schema migration.

**Architecture:** Layered Handler(Chi)→Service→Store(sqlc/pgx)→PostgreSQL. Public read routes switch `RequireAuth`→`OptionalAuth`, return lean response DTOs (never the raw sqlc row), and 404 non-active orgs. The header self-404s via a new `status='active'` query (mirror of Phase A `GetPublicProfileByUsername`); the Members tab (and future B2 content tabs) is protected by a shared `requireActiveOrgByID` middleware (mirror of Phase A `requireActiveProfileUser`). Integration tests run against real Postgres via `internal/testdb`.

**Tech Stack:** Go, Chi, sqlc + pgx/v5, PostgreSQL 18, `internal/testdb` harness (build tag `integration`).

## Global Constraints

- Multi-tenancy (ADR-004): every query filters `tenant_id`. Every new query MUST be tenant-scoped.
- Response contract: success `{"data": ...}` / lists `{"data": [...], "meta": {...}}`; errors `{"error":{"code","message"}}`; IDs as strings, timestamps RFC3339. `pgtype.UUID`/`pgtype.Timestamptz` already marshal to those forms — a lean DTO that is a subset of the sqlc row with the same field types is marshal-identical, only fewer fields.
- Public surfaces: `OptionalAuth` (guest-readable), lean DTOs (NO internal columns: `tenant_id`, `owner_user_id`, `field_values`, `status`, `deleted_at`, `invited_by`), and 404 for non-active/deleted orgs. Unverified-but-active orgs ARE viewable (Section 07); only the LIST filters to `verified`.
- Work-on-main-in-place: `git add` ONLY the exact files each task lists — never `git add .`. A global `sqlc generate` also regenerates unrelated dirty gen files (docs/reviews/tickets/onboarding/etc.); stage ONLY the gen file(s) this plan owns, by exact path.
- gopls diagnostics in this repo are frequently STALE — trust `go build ./...` (must exit 0), not the editor's inline errors.
- Do NOT include B2 (post-as-org migration, content/media/spaces tabs). This plan is read-surface only.

## File Structure

- `internal/testdb/seed.go` — MODIFY: add `OrgOpts`, `SeedOrganization`, `SeedOrgMembership`.
- `internal/db/queries/organizations.sql` — MODIFY: add `ActiveOrgExistsByID`, `GetPublicOrganizationBySlug`, `ListPublicOrganizationMembers`, `ListPublicOrganizations`/`CountPublicOrganizations`.
- `internal/db/gen/organizations.sql.go` — REGENERATED (stage this file only from the gen dir).
- `internal/organizations/store.go` — MODIFY: store methods wrapping the new queries.
- `internal/organizations/service.go` — MODIFY: service methods (incl. `ActiveOrgExists` for the gate).
- `internal/organizations/public_dto.go` — CREATE: `PublicOrgResponse`, `OrgMemberResponse`, `PublicOrgListItem` + mappers.
- `internal/organizations/public_handlers.go` — CREATE: `GetPublicOrganization`, `ListPublicMembers`, `ListPublicOrganizations` handlers.
- `internal/organizations/public_dto_test.go` — CREATE: mapper unit tests (non-integration).
- `internal/organizations/public_profile_integration_test.go` — CREATE: real-PG handler/query tests.
- `internal/server/org_gate.go` — CREATE: `requireActiveOrgByID` middleware.
- `internal/server/org_gate_integration_test.go` — CREATE: gate integration test.
- `internal/server/server.go` — MODIFY (lines ~1553-1557): repoint the three public org routes.

---

### Task 1: Test harness — org seeders

**Files:**
- Modify: `internal/testdb/seed.go`
- Test: `internal/testdb/testdb_smoke_test.go` (add one assertion)

**Interfaces:**
- Produces: `type OrgOpts struct { Slug, Name, Status, OrgType string; Verified bool; OwnerUserID pgtype.UUID }`; `SeedOrganization(t *testing.T, d *db.DB, tenantID pgtype.UUID, o OrgOpts) pgtype.UUID`; `SeedOrgMembership(t *testing.T, d *db.DB, orgID, userID pgtype.UUID, role string)`.
- Consumes: existing `SeedUser`, `mustUUID()`, `d.Writer.Exec`.

- [ ] **Step 1: Add the seeders to `internal/testdb/seed.go`**

Append (the file already has the `//go:build integration` tag and imports `uuid`, `pgtype`, `db`):

```go
// OrgOpts customizes SeedOrganization. Zero-value fields fall back to defaults:
// Slug/Name to a generated unique value, Status to "active", OrgType to
// "business". When OwnerUserID is not Valid, a throwaway active owner user is
// created (organizations.owner_user_id is NOT NULL REFERENCES users(id)).
type OrgOpts struct {
	Slug        string
	Name        string
	Status      string
	OrgType     string
	Verified    bool
	OwnerUserID pgtype.UUID
}

// SeedOrganization inserts an organizations row under tenantID and returns its id.
func SeedOrganization(t *testing.T, d *db.DB, tenantID pgtype.UUID, o OrgOpts) pgtype.UUID {
	t.Helper()

	ctx := t.Context()
	orgID := mustUUID()

	owner := o.OwnerUserID
	if !owner.Valid {
		owner = SeedUser(t, d, tenantID, UserOpts{})
	}
	slug := o.Slug
	if slug == "" {
		slug = "org-" + uuid.New().String()
	}
	name := o.Name
	if name == "" {
		name = slug
	}
	status := o.Status
	if status == "" {
		status = "active"
	}
	orgType := o.OrgType
	if orgType == "" {
		orgType = "business"
	}

	_, err := d.Writer.Exec(ctx,
		`INSERT INTO organizations (id, tenant_id, owner_user_id, name, slug, org_type, status, verified)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		orgID, tenantID, owner, name, slug, orgType, status, o.Verified)
	if err != nil {
		t.Fatalf("testdb: SeedOrganization: %v", err)
	}
	return orgID
}

// SeedOrgMembership inserts an organization_members row. role defaults to "member".
func SeedOrgMembership(t *testing.T, d *db.DB, orgID, userID pgtype.UUID, role string) {
	t.Helper()
	if role == "" {
		role = "member"
	}
	_, err := d.Writer.Exec(t.Context(),
		`INSERT INTO organization_members (organization_id, user_id, role) VALUES ($1, $2, $3)`,
		orgID, userID, role)
	if err != nil {
		t.Fatalf("testdb: SeedOrgMembership: %v", err)
	}
}
```

- [ ] **Step 2: Add a smoke assertion to `internal/testdb/testdb_smoke_test.go`**

Add this test function to the existing file:

```go
func TestSeedOrganizationSmoke(t *testing.T) {
	database := Connect(t)
	tenantID := NewTenant(t, database)

	orgID := SeedOrganization(t, database, tenantID, OrgOpts{Slug: "smoke-org", Status: "active"})
	owner := SeedUser(t, database, tenantID, UserOpts{Username: "smoke-owner"})
	SeedOrgMembership(t, database, orgID, owner, "admin")

	var status string
	if err := database.Reader.QueryRow(t.Context(),
		`SELECT status FROM organizations WHERE id = $1`, orgID).Scan(&status); err != nil {
		t.Fatalf("read org: %v", err)
	}
	if status != "active" {
		t.Fatalf("org status = %q, want active", status)
	}
}
```

- [ ] **Step 3: Run the smoke test**

Run: `go test -tags integration ./internal/testdb/ -run TestSeedOrganizationSmoke -v`
Expected: PASS.

- [ ] **Step 4: Build check**

Run: `go build ./... && go vet -tags integration ./internal/testdb/`
Expected: exit 0, no output.

- [ ] **Step 5: Commit**

```bash
git add internal/testdb/seed.go internal/testdb/testdb_smoke_test.go
git commit -m "test(harness): SeedOrganization + SeedOrgMembership seeders (Phase B1)"
```

---

### Task 2: Active-org gate (query + service + middleware)

**Files:**
- Modify: `internal/db/queries/organizations.sql`, `internal/organizations/store.go`, `internal/organizations/service.go`
- Regenerate: `internal/db/gen/organizations.sql.go`
- Create: `internal/server/org_gate.go`, `internal/server/org_gate_integration_test.go`

**Interfaces:**
- Produces: query `ActiveOrgExistsByID`; `Store.ActiveOrgExistsByID(ctx, tenantID, id pgtype.UUID) (bool, error)`; `Service.ActiveOrgExists(ctx, tenantID, id pgtype.UUID) (bool, error)`; middleware `requireActiveOrgByID(orgSvc *organizations.Service) func(http.Handler) http.Handler` (reads chi `{id}`, 404s non-active orgs).
- Consumes: `tenant.FromContext`, `chi.URLParam`, `organizations.Service`.

- [ ] **Step 1: Add the existence query to `internal/db/queries/organizations.sql`**

```sql
-- name: ActiveOrgExistsByID :one
-- Existence check for the public org profile gate: is there an ACTIVE,
-- non-deleted organization with this id in this tenant?
SELECT EXISTS (
    SELECT 1 FROM organizations
    WHERE tenant_id = @tenant_id AND id = @id
      AND status = 'active' AND deleted_at IS NULL
) AS exists;
```

- [ ] **Step 2: Regenerate sqlc**

Run: `sqlc generate`
Expected: exit 0. Only stage `internal/db/gen/organizations.sql.go` later — other gen files may show as modified from pre-existing dirty WIP; leave them.

- [ ] **Step 3: Add the store method to `internal/organizations/store.go`**

```go
func (s *Store) ActiveOrgExistsByID(ctx context.Context, tenantID, id pgtype.UUID) (bool, error) {
	return s.read.ActiveOrgExistsByID(ctx, gen.ActiveOrgExistsByIDParams{TenantID: tenantID, ID: id})
}
```

- [ ] **Step 4: Add the service method to `internal/organizations/service.go`**

```go
// ActiveOrgExists reports whether an ACTIVE, non-deleted org with this id
// exists in the tenant. Powers the public profile gate.
func (s *Service) ActiveOrgExists(ctx context.Context, tenantID, id pgtype.UUID) (bool, error) {
	return s.store.ActiveOrgExistsByID(ctx, tenantID, id)
}
```

- [ ] **Step 5: Create the gate `internal/server/org_gate.go`**

```go
package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/organizations"
	"github.com/jedafe/japa/internal/tenant"
)

// requireActiveOrgByID gates public org profile tabs keyed by {id}: it 404s
// unless {id} is an ACTIVE, non-deleted org in the request's tenant. Shared
// enforcement point mirroring requireActiveProfileUser. (The header route keys
// by {slug} and self-404s via GetPublicOrganizationBySlug instead.)
func requireActiveOrgByID(orgSvc *organizations.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := tenant.FromContext(r.Context())
			if t == nil {
				writeOrgGateError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
				return
			}

			var orgID pgtype.UUID
			if err := orgID.Scan(chi.URLParam(r, "id")); err != nil {
				writeOrgGateError(w, http.StatusBadRequest, "invalid_org_id", "Invalid organization ID format")
				return
			}

			exists, err := orgSvc.ActiveOrgExists(r.Context(), t.ID, orgID)
			if err != nil {
				writeOrgGateError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve organization")
				return
			}
			if !exists {
				writeOrgGateError(w, http.StatusNotFound, "org_not_found", "Organization not found")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeOrgGateError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
```

- [ ] **Step 6: Create the gate test `internal/server/org_gate_integration_test.go`**

```go
//go:build integration

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/jedafe/japa/internal/organizations"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/testdb"
)

func TestRequireActiveOrgByID(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	activeOrg := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "gate-active", Status: "active"})
	suspendedOrg := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "gate-suspended", Status: "suspended"})

	orgSvc := organizations.NewService(database)

	var nextCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := requireActiveOrgByID(orgSvc)(next)

	newReq := func(id string, withTenant bool) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/organizations/"+id+"/members", nil)
		if withTenant {
			req = req.WithContext(context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID}))
		}
		return testdb.WithChiURLParam(req, "id", id)
	}

	tests := []struct {
		name       string
		id         string
		withTenant bool
		wantStatus int
		wantNext   bool
	}{
		{"active org passes", activeOrg.String(), true, http.StatusOK, true},
		{"suspended org 404", suspendedOrg.String(), true, http.StatusNotFound, false},
		{"nonexistent org 404", uuid.New().String(), true, http.StatusNotFound, false},
		{"missing tenant 503", activeOrg.String(), false, http.StatusServiceUnavailable, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled = false
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, newReq(tc.id, tc.withTenant))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status=%d want %d; body=%s", rec.Code, tc.wantStatus, rec.Body)
			}
			if nextCalled != tc.wantNext {
				t.Fatalf("nextCalled=%v want %v", nextCalled, tc.wantNext)
			}
		})
	}
}
```

- [ ] **Step 7: Run tests + build**

Run: `go build ./... && go test -tags integration ./internal/server/ -run TestRequireActiveOrgByID -v`
Expected: build exit 0; test PASS (active→200, suspended→404, nonexistent→404, missing-tenant→503).

- [ ] **Step 8: Commit**

```bash
git add internal/db/queries/organizations.sql internal/db/gen/organizations.sql.go \
        internal/organizations/store.go internal/organizations/service.go \
        internal/server/org_gate.go internal/server/org_gate_integration_test.go
git commit -m "feat(org): shared active-org gate for public org profile (Phase B1)"
```

---

### Task 3: Public org header (lean DTO + query + route)

**Files:**
- Modify: `internal/db/queries/organizations.sql`, `internal/organizations/store.go`, `internal/organizations/service.go`, `internal/server/server.go`
- Regenerate: `internal/db/gen/organizations.sql.go`
- Create: `internal/organizations/public_dto.go`, `internal/organizations/public_handlers.go`, `internal/organizations/public_dto_test.go`, `internal/organizations/public_profile_integration_test.go`

**Interfaces:**
- Produces: query `GetPublicOrganizationBySlug`; `Store.GetPublicOrganizationBySlug(ctx, slug string, tenantID pgtype.UUID) (gen.Organization, error)`; `Service.GetPublicOrganizationBySlug(...)`; `PublicOrgResponse` + `toPublicOrg(gen.Organization) PublicOrgResponse`; handler `Handlers.GetPublicOrganization`.
- Consumes: existing helpers `requireTenantID`, `writeJSON`, `handleServiceError`, `chi.URLParam`.

- [ ] **Step 1: Add the public header query to `internal/db/queries/organizations.sql`**

```sql
-- name: GetPublicOrganizationBySlug :one
-- Public org profile header: only ACTIVE, non-deleted orgs (unverified-but-
-- active is a valid public state per Section 07). Non-active/deleted → no row → 404.
SELECT * FROM organizations
WHERE slug = @slug AND tenant_id = @tenant_id
  AND status = 'active' AND deleted_at IS NULL;
```

- [ ] **Step 2: Regenerate sqlc**

Run: `sqlc generate`
Expected: exit 0.

- [ ] **Step 3: Add the store + service methods**

`internal/organizations/store.go`:
```go
func (s *Store) GetPublicOrganizationBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.Organization, error) {
	return s.read.GetPublicOrganizationBySlug(ctx, gen.GetPublicOrganizationBySlugParams{Slug: slug, TenantID: tenantID})
}
```
`internal/organizations/service.go`:
```go
// GetPublicOrganizationBySlug returns an active org for the public profile header.
func (s *Service) GetPublicOrganizationBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.Organization, error) {
	return s.store.GetPublicOrganizationBySlug(ctx, slug, tenantID)
}
```

- [ ] **Step 4: Write the failing mapper test `internal/organizations/public_dto_test.go`**

```go
package organizations

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}
	return u
}

func TestToPublicOrg_LeanAndCorrect(t *testing.T) {
	tagline := "we help"
	row := gen.Organization{
		ID:            mustUUID(t, "11111111-1111-1111-1111-111111111111"),
		TenantID:      mustUUID(t, "22222222-2222-2222-2222-222222222222"),
		OwnerUserID:   mustUUID(t, "33333333-3333-3333-3333-333333333333"),
		Name:          "Acme Corp",
		Slug:          "acme",
		Tagline:       &tagline,
		OrgType:       "business",
		Verified:      true,
		FollowerCount: 12,
		MemberCount:   4,
		Status:        "active",
		FieldValues:   []byte(`{"secret":"x"}`),
		CreatedAt:     pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	c := toPublicOrg(row)
	if c.Name != "Acme Corp" || c.Slug != "acme" || !c.Verified || c.MemberCount != 4 {
		t.Fatalf("field-copy mismatch: %+v", c)
	}

	blob, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(blob)
	for _, leaked := range []string{"tenant_id", "owner_user_id", "field_values", "\"status\"", "deleted_at", "updated_at"} {
		if strings.Contains(got, leaked) {
			t.Errorf("PublicOrgResponse leaks %q: %s", leaked, got)
		}
	}
	for _, want := range []string{"\"id\"", "\"name\"", "\"slug\"", "member_count", "verified"} {
		if !strings.Contains(got, want) {
			t.Errorf("PublicOrgResponse missing %q: %s", want, got)
		}
	}
}
```

- [ ] **Step 5: Run the mapper test to verify it fails**

Run: `go test ./internal/organizations/ -run TestToPublicOrg_LeanAndCorrect -v`
Expected: FAIL — `undefined: toPublicOrg`.

- [ ] **Step 6: Create `internal/organizations/public_dto.go`**

```go
package organizations

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// PublicOrgResponse is the lean public shape for the org profile header. It
// omits internal columns the raw gen.Organization row carries (tenant_id,
// owner_user_id, field_values, status, deleted_at, updated_at).
type PublicOrgResponse struct {
	ID               pgtype.UUID        `json:"id"`
	Name             string             `json:"name"`
	Slug             string             `json:"slug"`
	Tagline          *string            `json:"tagline"`
	Description      *string            `json:"description"`
	OrgType          string             `json:"org_type"`
	LogoMediaID      pgtype.UUID        `json:"logo_media_id"`
	HeaderMediaID    pgtype.UUID        `json:"header_media_id"`
	WebsiteURL       *string            `json:"website_url"`
	CountryCode      *string            `json:"country_code"`
	Verified         bool               `json:"verified"`
	VerificationType *string            `json:"verification_type"`
	FollowerCount    int32              `json:"follower_count"`
	MemberCount      int32              `json:"member_count"`
	CreatedAt        pgtype.Timestamptz `json:"created_at"`
}

func toPublicOrg(o gen.Organization) PublicOrgResponse {
	return PublicOrgResponse{
		ID:               o.ID,
		Name:             o.Name,
		Slug:             o.Slug,
		Tagline:          o.Tagline,
		Description:      o.Description,
		OrgType:          o.OrgType,
		LogoMediaID:      o.LogoMediaID,
		HeaderMediaID:    o.HeaderMediaID,
		WebsiteURL:       o.WebsiteUrl,
		CountryCode:      o.CountryCode,
		Verified:         o.Verified,
		VerificationType: o.VerificationType,
		FollowerCount:    o.FollowerCount,
		MemberCount:      o.MemberCount,
		CreatedAt:        o.CreatedAt,
	}
}
```

- [ ] **Step 7: Run the mapper test to verify it passes**

Run: `go test ./internal/organizations/ -run TestToPublicOrg_LeanAndCorrect -v`
Expected: PASS.

- [ ] **Step 8: Create the public header handler in `internal/organizations/public_handlers.go`**

```go
package organizations

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GetPublicOrganization handles GET /organizations/{slug} (public). Returns the
// lean org header for ACTIVE orgs; non-active/deleted/missing → 404 (the query
// returns no row → handleServiceError maps pgx.ErrNoRows to 404).
func (h *Handlers) GetPublicOrganization(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	org, err := h.service.GetPublicOrganizationBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": toPublicOrg(org)})
}
```

- [ ] **Step 9: Repoint the header route in `internal/server/server.go`**

Change line ~1554 from:
```go
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/organizations/{slug}", orgHandlers.GetOrganization)
```
to:
```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/organizations/{slug}", orgHandlers.GetPublicOrganization)
```

- [ ] **Step 10: Write the header integration test in `internal/organizations/public_profile_integration_test.go`**

```go
//go:build integration

package organizations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/testdb"
)

func TestGetPublicOrganization_ActiveLean_NonActive404(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active"})
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "shady", Status: "suspended"})

	h := NewHandlers(NewService(database))

	call := func(slug string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/organizations/"+slug, nil)
		req = req.WithContext(context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID}))
		req = testdb.WithChiURLParam(req, "slug", slug)
		rec := httptest.NewRecorder()
		h.GetPublicOrganization(rec, req)
		return rec
	}

	// active → 200 lean (no internal columns)
	rec := call("acme")
	if rec.Code != http.StatusOK {
		t.Fatalf("active: status=%d want 200; body=%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, leaked := range []string{"tenant_id", "owner_user_id", "field_values"} {
		if strings.Contains(body, leaked) {
			t.Errorf("active org body leaks %q: %s", leaked, body)
		}
	}
	var envelope struct{ Data map[string]any `json:"data"` }
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.Data["slug"] != "acme" {
		t.Fatalf("slug=%v want acme", envelope.Data["slug"])
	}

	// suspended → 404
	if rec := call("shady"); rec.Code != http.StatusNotFound {
		t.Fatalf("suspended: status=%d want 404; body=%s", rec.Code, rec.Body)
	}
	// missing → 404
	if rec := call("nope"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing: status=%d want 404; body=%s", rec.Code, rec.Body)
	}
}
```

- [ ] **Step 11: Run header tests + build**

Run: `go build ./... && go test -tags integration ./internal/organizations/ -run 'TestToPublicOrg_LeanAndCorrect|TestGetPublicOrganization_ActiveLean_NonActive404' -v`
Expected: build exit 0; both PASS.

- [ ] **Step 12: Commit**

```bash
git add internal/db/queries/organizations.sql internal/db/gen/organizations.sql.go \
        internal/organizations/store.go internal/organizations/service.go \
        internal/organizations/public_dto.go internal/organizations/public_handlers.go \
        internal/organizations/public_dto_test.go internal/organizations/public_profile_integration_test.go \
        internal/server/server.go
git commit -m "feat(org): public org header — OptionalAuth + lean PublicOrgResponse (Phase B1)"
```

---

### Task 4: Public Members tab (lean member DTO + active-only query + route)

**Files:**
- Modify: `internal/db/queries/organizations.sql`, `internal/organizations/store.go`, `internal/organizations/service.go`, `internal/organizations/public_dto.go`, `internal/organizations/public_handlers.go`, `internal/organizations/public_dto_test.go`, `internal/organizations/public_profile_integration_test.go`, `internal/server/server.go`
- Regenerate: `internal/db/gen/organizations.sql.go`

**Interfaces:**
- Produces: query `ListPublicOrganizationMembers`; `Store.ListPublicMembers(ctx, orgID, tenantID pgtype.UUID) ([]gen.ListPublicOrganizationMembersRow, error)`; `Service.ListPublicMembers(...)`; `OrgMemberResponse` + `toOrgMembers(...)`; handler `Handlers.ListPublicMembers`.
- Consumes: `requireActiveOrgByID` (Task 2), `requireTenantID`, `parseUUIDParam`, `writeJSON`.

- [ ] **Step 1: Add the active-members query to `internal/db/queries/organizations.sql`**

```sql
-- name: ListPublicOrganizationMembers :many
-- Public Members tab: ACTIVE members only (banned/suspended/deleted users
-- excluded), tenant-scoped via the users join (defense-in-depth). Excludes the
-- internal invited_by column by projecting only public fields.
SELECT om.organization_id, om.user_id, om.role, om.joined_at,
       u.username, up.display_name, up.avatar_url
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.tenant_id = @tenant_id AND u.status = 'active'
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = u.tenant_id
WHERE om.organization_id = @organization_id
ORDER BY om.joined_at DESC;
```

- [ ] **Step 2: Regenerate sqlc**

Run: `sqlc generate`
Expected: exit 0.

- [ ] **Step 3: Add store + service methods**

`internal/organizations/store.go`:
```go
func (s *Store) ListPublicMembers(ctx context.Context, orgID, tenantID pgtype.UUID) ([]gen.ListPublicOrganizationMembersRow, error) {
	return s.read.ListPublicOrganizationMembers(ctx, gen.ListPublicOrganizationMembersParams{OrganizationID: orgID, TenantID: tenantID})
}
```
`internal/organizations/service.go`:
```go
// ListPublicMembers returns the org's ACTIVE members for the public Members tab.
func (s *Service) ListPublicMembers(ctx context.Context, orgID, tenantID pgtype.UUID) ([]gen.ListPublicOrganizationMembersRow, error) {
	return s.store.ListPublicMembers(ctx, orgID, tenantID)
}
```

- [ ] **Step 4: Write the failing member-mapper test — add to `internal/organizations/public_dto_test.go`**

```go
func TestToOrgMembers_LeanAndCorrect(t *testing.T) {
	dn := "Jane Doe"
	rows := []gen.ListPublicOrganizationMembersRow{{
		OrganizationID: mustUUID(t, "11111111-1111-1111-1111-111111111111"),
		UserID:         mustUUID(t, "22222222-2222-2222-2222-222222222222"),
		Role:           "admin",
		JoinedAt:       pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Username:       "jane",
		DisplayName:    &dn,
	}}

	cards := toOrgMembers(rows)
	if len(cards) != 1 || cards[0].Role != "admin" || cards[0].User.Username != "jane" {
		t.Fatalf("field-copy mismatch: %+v", cards)
	}

	blob, _ := json.Marshal(cards)
	got := string(blob)
	if strings.Contains(got, "invited_by") || strings.Contains(got, "organization_id") {
		t.Errorf("OrgMemberResponse leaks internal field: %s", got)
	}
	for _, want := range []string{"\"role\"", "\"user\"", "username", "joined_at"} {
		if !strings.Contains(got, want) {
			t.Errorf("OrgMemberResponse missing %q: %s", want, got)
		}
	}
}
```
(Add `"strings"` and `"time"` to the test file imports if not already present from Task 3.)

- [ ] **Step 5: Run to verify it fails**

Run: `go test ./internal/organizations/ -run TestToOrgMembers_LeanAndCorrect -v`
Expected: FAIL — `undefined: toOrgMembers`.

- [ ] **Step 6: Add the member DTO + mapper to `internal/organizations/public_dto.go`**

```go
// OrgMemberUser is the lean public user shape embedded in a member row.
type OrgMemberUser struct {
	ID          pgtype.UUID `json:"id"`
	Username    string      `json:"username"`
	DisplayName *string     `json:"display_name"`
	AvatarURL   *string     `json:"avatar_url"`
}

// OrgMemberResponse is the lean public shape for the Members tab: the member's
// public user identity plus their org role. Omits invited_by and organization_id.
type OrgMemberResponse struct {
	User     OrgMemberUser      `json:"user"`
	Role     string             `json:"role"`
	JoinedAt pgtype.Timestamptz `json:"joined_at"`
}

func toOrgMembers(rows []gen.ListPublicOrganizationMembersRow) []OrgMemberResponse {
	out := make([]OrgMemberResponse, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = OrgMemberResponse{
			User: OrgMemberUser{
				ID:          r.UserID,
				Username:    r.Username,
				DisplayName: r.DisplayName,
				AvatarURL:   r.AvatarUrl,
			},
			Role:     r.Role,
			JoinedAt: r.JoinedAt,
		}
	}
	return out
}
```

- [ ] **Step 7: Run to verify it passes**

Run: `go test ./internal/organizations/ -run TestToOrgMembers_LeanAndCorrect -v`
Expected: PASS.

- [ ] **Step 8: Add the handler to `internal/organizations/public_handlers.go`**

```go
// ListPublicMembers handles GET /organizations/{id}/members (public). The
// active-org gate (requireActiveOrgByID) has already 404'd non-active orgs.
func (h *Handlers) ListPublicMembers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}
	orgID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	rows, err := h.service.ListPublicMembers(r.Context(), orgID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toOrgMembers(rows)})
}
```

- [ ] **Step 9: Repoint the members route in `internal/server/server.go`**

Change line ~1557 from:
```go
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/organizations/{id}/members", orgHandlers.ListMembers)
```
to:
```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Get("/organizations/{id}/members", orgHandlers.ListPublicMembers)
```

- [ ] **Step 10: Add the members integration test to `internal/organizations/public_profile_integration_test.go`**

```go
func TestListPublicMembers_ActiveOnly_Lean(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active"})

	activeUser := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "active-mem", DisplayName: "Active Mem"})
	suspendedUser := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "susp-mem", Status: "suspended"})
	testdb.SeedOrgMembership(t, database, orgID, activeUser, "admin")
	testdb.SeedOrgMembership(t, database, orgID, suspendedUser, "member")

	h := NewHandlers(NewService(database))
	req := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID.String()+"/members", nil)
	req = req.WithContext(context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID}))
	req = testdb.WithChiURLParam(req, "id", orgID.String())
	rec := httptest.NewRecorder()
	h.ListPublicMembers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "invited_by") {
		t.Errorf("members body leaks invited_by: %s", body)
	}
	var envelope struct {
		Data []struct {
			Role string `json:"role"`
			User struct{ Username string `json:"username"` } `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(envelope.Data) != 1 {
		t.Fatalf("got %d members, want 1 (active only)", len(envelope.Data))
	}
	if envelope.Data[0].User.Username != "active-mem" || envelope.Data[0].Role != "admin" {
		t.Fatalf("unexpected member: %+v", envelope.Data[0])
	}
}
```
(Ensure `"strings"` is imported in this file.)

- [ ] **Step 11: Run members tests + build**

Run: `go build ./... && go test -tags integration ./internal/organizations/ -run 'TestToOrgMembers_LeanAndCorrect|TestListPublicMembers_ActiveOnly_Lean' -v`
Expected: build exit 0; both PASS.

- [ ] **Step 12: Commit**

```bash
git add internal/db/queries/organizations.sql internal/db/gen/organizations.sql.go \
        internal/organizations/store.go internal/organizations/service.go \
        internal/organizations/public_dto.go internal/organizations/public_handlers.go \
        internal/organizations/public_dto_test.go internal/organizations/public_profile_integration_test.go \
        internal/server/server.go
git commit -m "feat(org): public Members tab — active members, lean DTO, gated (Phase B1)"
```

---

### Task 5: Public org list (verified+active, lean) + catalog

**Files:**
- Modify: `internal/db/queries/organizations.sql`, `internal/organizations/store.go`, `internal/organizations/service.go`, `internal/organizations/public_dto.go`, `internal/organizations/public_handlers.go`, `internal/organizations/public_profile_integration_test.go`, `internal/server/server.go`
- Regenerate: `internal/db/gen/organizations.sql.go`
- Modify (submodule): `.jeos/conventions/api-routes-catalog.md`

**Interfaces:**
- Produces: query `ListPublicOrganizations` + `CountPublicOrganizations`; `Store.ListPublicOrganizations(ctx, tenantID pgtype.UUID, limit, offset int32) (...)` and count; `Service.ListPublicOrganizations(...)`; `PublicOrgListItem` + `toPublicOrgList(...)`; handler `Handlers.ListPublicOrganizations`.
- Consumes: `requireTenantID`, `writeJSON`, existing `parsePageSize`/offset helpers in the org package (verify names; the org handler already paginates `ListOrganizations` — reuse whatever it uses).

- [ ] **Step 1: Confirm the org package's pagination helper names**

Run: `grep -nE 'parsePage|Limit|Offset|strconv' internal/organizations/handlers.go | head`
Use the same helpers the existing `ListOrganizations` handler (handlers.go:181) uses. If it parses `limit`/`offset` inline, mirror that exactly in Step 6.

- [ ] **Step 2: Add the public list queries to `internal/db/queries/organizations.sql`**

```sql
-- name: ListPublicOrganizations :many
-- Public Discover "Organizations": VERIFIED + ACTIVE orgs only, newest first.
SELECT id, name, slug, tagline, org_type, logo_media_id, header_media_id,
       country_code, verified, follower_count, member_count, created_at
FROM organizations
WHERE tenant_id = @tenant_id AND status = 'active' AND deleted_at IS NULL AND verified = true
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPublicOrganizations :one
SELECT COUNT(*) FROM organizations
WHERE tenant_id = @tenant_id AND status = 'active' AND deleted_at IS NULL AND verified = true;
```

- [ ] **Step 3: Regenerate sqlc**

Run: `sqlc generate`
Expected: exit 0.

- [ ] **Step 4: Add store + service methods**

`internal/organizations/store.go`:
```go
func (s *Store) ListPublicOrganizations(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ListPublicOrganizationsRow, error) {
	return s.read.ListPublicOrganizations(ctx, gen.ListPublicOrganizationsParams{TenantID: tenantID, PageLimit: limit, PageOffset: offset})
}

func (s *Store) CountPublicOrganizations(ctx context.Context, tenantID pgtype.UUID) (int64, error) {
	return s.read.CountPublicOrganizations(ctx, tenantID)
}
```
`internal/organizations/service.go`:
```go
// ListPublicOrganizations returns verified+active orgs for the public Discover list.
func (s *Service) ListPublicOrganizations(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ListPublicOrganizationsRow, int64, error) {
	rows, err := s.store.ListPublicOrganizations(ctx, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountPublicOrganizations(ctx, tenantID)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
```

- [ ] **Step 5: Add the list-item DTO + mapper to `internal/organizations/public_dto.go`**

```go
// PublicOrgListItem is the lean shape for the public Discover org list.
type PublicOrgListItem struct {
	ID            pgtype.UUID        `json:"id"`
	Name          string             `json:"name"`
	Slug          string             `json:"slug"`
	Tagline       *string            `json:"tagline"`
	OrgType       string             `json:"org_type"`
	LogoMediaID   pgtype.UUID        `json:"logo_media_id"`
	HeaderMediaID pgtype.UUID        `json:"header_media_id"`
	CountryCode   *string            `json:"country_code"`
	Verified      bool               `json:"verified"`
	FollowerCount int32              `json:"follower_count"`
	MemberCount   int32              `json:"member_count"`
	CreatedAt     pgtype.Timestamptz `json:"created_at"`
}

func toPublicOrgList(rows []gen.ListPublicOrganizationsRow) []PublicOrgListItem {
	out := make([]PublicOrgListItem, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = PublicOrgListItem{
			ID:            r.ID,
			Name:          r.Name,
			Slug:          r.Slug,
			Tagline:       r.Tagline,
			OrgType:       r.OrgType,
			LogoMediaID:   r.LogoMediaID,
			HeaderMediaID: r.HeaderMediaID,
			CountryCode:   r.CountryCode,
			Verified:      r.Verified,
			FollowerCount: r.FollowerCount,
			MemberCount:   r.MemberCount,
			CreatedAt:     r.CreatedAt,
		}
	}
	return out
}
```

- [ ] **Step 6: Add the handler to `internal/organizations/public_handlers.go`**

```go
// ListPublicOrganizations handles GET /organizations (public): verified+active
// orgs, paginated. Uses the same limit/offset parsing as the existing
// ListOrganizations handler (mirror it exactly — see Step 1).
func (h *Handlers) ListPublicOrganizations(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	limit, offset := publicListPaging(r)

	rows, total, err := h.service.ListPublicOrganizations(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": toPublicOrgList(rows),
		"meta": map[string]any{"total": total, "limit": limit, "offset": offset},
	})
}

// publicListPaging parses ?limit=&offset= for the public org list: default
// limit 20, capped at 100; default offset 0. (If the org package already has an
// equivalent shared helper — check Step 1 — call it and delete this one.)
func publicListPaging(r *http.Request) (int32, int32) {
	limit := int32(20)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 100 {
				n = 100
			}
			limit = int32(n)
		}
	}
	var offset int32
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			offset = int32(n)
		}
	}
	return limit, offset
}
```
Add `"strconv"` to `public_handlers.go` imports for this. (`public_handlers.go` already imports `net/http` and `github.com/go-chi/chi/v5`.)

- [ ] **Step 7: Repoint the list route in `internal/server/server.go`**

Change line ~1553 from:
```go
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/organizations", orgHandlers.ListOrganizations)
```
to:
```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/organizations", orgHandlers.ListPublicOrganizations)
```
(The authenticated/admin `ListOrganizations` handler stays in the file for admin use if wired elsewhere; this route becomes the public list.)

- [ ] **Step 8: Add the list integration test to `internal/organizations/public_profile_integration_test.go`**

```go
func TestListPublicOrganizations_VerifiedActiveOnly(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "v1", Status: "active", Verified: true})
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "v2", Status: "active", Verified: true})
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "unverified", Status: "active", Verified: false})
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "suspended", Status: "suspended", Verified: true})

	h := NewHandlers(NewService(database))
	req := httptest.NewRequest(http.MethodGet, "/organizations", nil)
	req = req.WithContext(context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID}))
	rec := httptest.NewRecorder()
	h.ListPublicOrganizations(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body)
	}
	var envelope struct {
		Data []struct{ Slug string `json:"slug"` } `json:"data"`
		Meta struct{ Total int `json:"total"` } `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.Meta.Total != 2 || len(envelope.Data) != 2 {
		t.Fatalf("got total=%d len=%d, want 2 verified+active only", envelope.Meta.Total, len(envelope.Data))
	}
}
```

- [ ] **Step 9: Run list tests + build**

Run: `go build ./... && go test -tags integration ./internal/organizations/ -run TestListPublicOrganizations_VerifiedActiveOnly -v`
Expected: build exit 0; PASS.

- [ ] **Step 10: Update the API catalog (submodule)**

In `.jeos/conventions/api-routes-catalog.md`, under the Organizations section, annotate the three now-public routes and note the gate. Example edits:
```
GET    /organizations                       [public] verified+active orgs (Discover)
GET    /organizations/{slug}                [public] lean org header (active only, no PII)
GET    /organizations/{id}/members          [public] active members, gated by requireActiveOrgByID
```
Commit inside the submodule:
```bash
cd .jeos && git add conventions/api-routes-catalog.md && \
  git commit -m "docs(api-catalog): public org read surface (header/members/list) (Phase B1)" && cd ..
```
(The outer gitlink is left modified for the cron bundler; the submodule commit will be pushed to `japaconnect-engineering` at finish time, before the outer main advances the gitlink.)

- [ ] **Step 11: Commit the code**

```bash
git add internal/db/queries/organizations.sql internal/db/gen/organizations.sql.go \
        internal/organizations/store.go internal/organizations/service.go \
        internal/organizations/public_dto.go internal/organizations/public_handlers.go \
        internal/organizations/public_profile_integration_test.go \
        internal/server/server.go
git commit -m "feat(org): public org list — verified+active, lean, OptionalAuth (Phase B1)"
```

---

## Final verification (after all tasks)

- [ ] `go build ./...` → exit 0
- [ ] `go vet -tags integration ./internal/organizations/ ./internal/server/ ./internal/testdb/` → exit 0
- [ ] `go test -tags integration -p 1 ./internal/organizations/ ./internal/server/ ./internal/testdb/` → all PASS (run serially: `-p 1` avoids the known cross-package contention on the shared `japa_test` DB).
- [ ] Confirm the three org routes now read `OptionalAuth` (grep `internal/server/server.go` for `/organizations`).
- [ ] Pre-existing `TestNoDuplicateRoutes`/`TestNoOverlappingParamRoutes` remain red from cron bundle `06dd6cc` — NOT introduced by this plan; confirm no NEW collision from the repointed routes (same paths, only middleware/handler changed).

## Notes for the executor (work-on-main hazards)

- A cron auto-committer sweeps the whole tree and may bundle your uncommitted files into its own commit. Commit each task immediately. If `git status` shows a task's files already clean (cron grabbed them), verify they are in HEAD's tree and proceed.
- `sqlc generate` is global: after each regen, `git add` ONLY `internal/db/gen/organizations.sql.go` — never the whole gen dir.
- Do NOT `git add .`. Stage the exact paths each task lists.
