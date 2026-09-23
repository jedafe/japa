# Agent Data API v1 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose JapaConnect's structured migration data (visa routes, timelines, credentials, language tests, scholarships, universities) through the §47 public API facade with freemium tier gating, a flagship destination-brief endpoint, and hardened MCP API-key access.

**Architecture:** Extend `internal/publicapi` with narrow reader interfaces (`MoatReaders`) that delegate to existing domain services — never re-implementing domain queries. REST handlers return domain DTOs under a consistent `{data,total,limit,offset}` envelope. Premium endpoints are gated by a new `RequireTier` middleware that resolves the key owner's unified tier per request. MCP gets API-key rate limiting and a read-only tool allowlist; new moat tools are backed by the same `publicapi.Service` methods.

**Tech Stack:** Go, chi router, pgx/v5, sqlc `gen.Queries`, Valkey (asynq/valkey-go), errgroup, existing `internal/errors` envelope.

**Spec:** `docs/superpowers/specs/2026-09-23-agent-data-api-design.md`

## Global Constraints

- **JEOS first** (AGENTS.md): catalog entries (routes, Valkey keys, system settings) are added to `.jeos/conventions/` before the code that uses them. Task 1 does this.
- **Rule 3:** No hardcoded tunables — read `api.destination_brief.cache_ttl_seconds` via the settings-reader interface (fallback 900).
- **Rule 4:** Valkey keys use `valkey.MustKey` (emits `jc:` prefix); the new pattern `api_dest_brief:{slug}:{hash}` must exist in `valkey-keys-catalog.md` first.
- **Rule 5:** All errors via `errpkg.WriteError(w, status, "snake_case_code", msg)` / `errpkg.WriteJSON`.
- **Rule 17:** No AI attribution anywhere — no "Generated with", no co-author trailers, no `// AI:` comments.
- All `/public/*` routes stay inside the existing API-key group in `internal/server/server.go` (~line 4409) to avoid chi radix shadowing.
- Every new `/public/*` route gets `publicapi.RequireScope(...)`; premium routes additionally get `publicapi.RequireTier(publicAPIService, "pro")`; every domain route group gets `features.Gate(featureChecker, "<domain>.enabled")`.
- Tests follow the stub-based `httptest` pattern from `internal/mcp/mcp_test.go` — no testdb, no pgxmock.
- Commit to `main`. Do not create branches.

---

### Task 1: JEOS catalog updates (docs before code)

**Files:**
- Modify: `.jeos/conventions/api-routes-catalog.md` (append to the `/api/v1/public/*` block near line 925)
- Modify: `.jeos/conventions/valkey-keys-catalog.md`
- Modify: `.jeos/conventions/system-settings-catalog.md`
- Modify: `.jeos/sections/47-public-api-developer-portal.md` (append a "v1.1 Agent Data API" subsection)
- Modify: `.jeos/sections/119-mcp-server.md` (note API-key read-only allowlist)

- [ ] **Step 1: Add route entries to `api-routes-catalog.md`**

Append after the existing `/api/v1/public/trending/*` lines (~line 936):

```markdown
GET    /api/v1/public/visa-routes                     list visa routes (api.read.visa, visa_routes.enabled)
GET    /api/v1/public/visa-routes/{id}                get a visa route (api.read.visa, visa_routes.enabled)
GET    /api/v1/public/visa-routes/compare             compare visa routes by IDs (api.read.visa, pro tier, visa_routes.enabled)
GET    /api/v1/public/visa-timelines/stats            processing-time stats (api.read.visa, visa_timeline.enabled)
GET    /api/v1/public/visa-timelines/stats/trend      processing-time trend (api.read.visa, pro tier, visa_timeline.enabled)
GET    /api/v1/public/credentials                     list credential pathways (api.read.visa, credential_checker.enabled)
GET    /api/v1/public/credentials/{id}                get a credential pathway (api.read.visa, credential_checker.enabled)
GET    /api/v1/public/credentials/professions         list professions (api.read.visa, credential_checker.enabled)
GET    /api/v1/public/language-tests                  list language tests (api.read.visa, language_test_hub.enabled)
GET    /api/v1/public/language-tests/{slug}           get a language test (api.read.visa, language_test_hub.enabled)
GET    /api/v1/public/language-tests/requirements     list test requirements (api.read.visa, language_test_hub.enabled)
GET    /api/v1/public/scholarships                    search scholarships (api.read.education, scholarship_finder.enabled)
GET    /api/v1/public/scholarships/{id}               get scholarship detail (api.read.education, scholarship_finder.enabled)
GET    /api/v1/public/scholarships/statuses           list scholarship statuses (api.read.education, scholarship_finder.enabled)
GET    /api/v1/public/scholarships/levels             list study levels (api.read.education, scholarship_finder.enabled)
GET    /api/v1/public/universities                    list universities (api.read.education)
GET    /api/v1/public/universities/{slug}             get a university (api.read.education)
GET    /api/v1/public/destinations/{slug}/brief       aggregate destination brief (api.read.brief, pro tier)
```

- [ ] **Step 2: Add the Valkey key to `valkey-keys-catalog.md`**

Append a row matching the existing table format (mirroring the `jc:api_rate:` row at line 12):

```markdown
| `jc:api_dest_brief:{space_slug}:{include_hash}` | string (JSON-serialized DestinationBrief) | `api.destination_brief.cache_ttl_seconds` (default 900s) | explicit TTL |
```

- [ ] **Step 3: Add the system setting to `system-settings-catalog.md`**

Append near the `api.rate_limit.*` rows (~line 114):

```markdown
| `api.destination_brief.cache_ttl_seconds` | `900` | int | Destination-brief response cache TTL (seconds) |
```

- [ ] **Step 4: Update `.jeos/sections/47-public-api-developer-portal.md`**

Append a subsection documenting the v1.1 expansion: the three new scopes
(`api.read.visa`, `api.read.education`, `api.read.brief`), `agent.use` now in
ScopeCatalog, the `RequireTier` middleware (`regular→free`, `pro`/`business`→
premium), and the destination-brief endpoint contract.

- [ ] **Step 5: Update `.jeos/sections/119-mcp-server.md`**

Note: API-key-authenticated MCP callers are restricted to a read-only tool
allowlist (`content.read`, `content.search`, `rag.search`, `spaces.find`,
`spaces.read`, `spaces.list`, `scholarship.search`, `visa.search`, `visa.get`,
`visa.compare`, `university.get`, `destination.brief`) and consume the key's
sliding-window rate-limit budget. JWT callers are unchanged (full RBAC
catalog).

- [ ] **Step 6: Commit**

```bash
git add .jeos/conventions/api-routes-catalog.md .jeos/conventions/valkey-keys-catalog.md .jeos/conventions/system-settings-catalog.md .jeos/sections/47-public-api-developer-portal.md .jeos/sections/119-mcp-server.md
git commit -m "docs(jeos): catalog agent data API routes, brief cache key, and TTL setting"
```

---

### Task 2: Scope catalog + `RequireTier` middleware

**Files:**
- Modify: `internal/publicapi/scopes.go` (append 4 entries to `ScopeCatalog`)
- Create: `internal/publicapi/tier.go`
- Test: `internal/publicapi/tier_test.go`

**Interfaces:**
- Produces: `RequireTier(tr TierResolver, minTier string) func(http.Handler) http.Handler` — consumed by server.go wiring in Tasks 4–6.
- `publicapi.Service` already implements `TierResolver` (`ResolveUserTier`, service.go:344), so wiring passes `publicAPIService`.

- [ ] **Step 1: Write the failing test**

`internal/publicapi/tier_test.go`:

```go
package publicapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type stubTierResolver struct{ tier string }

func (s stubTierResolver) ResolveUserTier(_ context.Context, _ pgtype.UUID) (string, error) {
	return s.tier, nil
}

func requireTierReq(tier string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/public/x", nil)
	return r.WithContext(context.WithValue(r.Context(), apiKeyContextKey, &APIKeyContext{UserID: pgtype.UUID{Valid: true}}))
}

func TestRequireTier(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	cases := []struct {
		name     string
		userTier string
		minTier  string
		want     int
	}{
		{"regular denied pro endpoint", "regular", "pro", http.StatusForbidden},
		{"pro allowed pro endpoint", "pro", "pro", http.StatusNoContent},
		{"business allowed pro endpoint", "business", "pro", http.StatusNoContent},
		{"unknown tier denied", "gold", "pro", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			RequireTier(stubTierResolver{tier: c.userTier}, c.minTier)(ok).ServeHTTP(rec, requireTierReq(c.userTier))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d", rec.Code, c.want)
			}
		})
	}

	t.Run("no api key -> 401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/public/x", nil)
		RequireTier(stubTierResolver{tier: "pro"}, "pro")(ok).ServeHTTP(rec, r)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/publicapi/ -run TestRequireTier -v`
Expected: FAIL — `undefined: RequireTier`

- [ ] **Step 3: Implement `tier.go`**

```go
package publicapi

import (
	"net/http"

	errpkg "github.com/jedafe/japa/internal/errors"
)

// tierRanks orders unified user tiers lowest to highest. Unknown tiers
// resolve to rank 0 — denied by any positive minimum (fail-closed).
var tierRanks = map[string]int{
	"regular":  1,
	"pro":      2,
	"business": 3,
}

// RequireTier enforces that the API key owner's unified user tier is at
// least minTier. The tier is resolved per request via tr so a lapsed
// subscription revokes access immediately without key revocation.
// Ref: docs/superpowers/specs/2026-09-23-agent-data-api-design.md (Tier gating)
func RequireTier(tr TierResolver, minTier string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyCtx := APIKeyFromContext(r.Context())
			if keyCtx == nil {
				errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
				return
			}

			tier := "regular"
			if tr != nil {
				if resolved, err := tr.ResolveUserTier(r.Context(), keyCtx.UserID); err == nil && resolved != "" {
					tier = resolved
				}
			}

			if tierRanks[tier] < tierRanks[minTier] {
				errpkg.WriteError(w, http.StatusForbidden, "tier_required",
					"This endpoint requires a "+minTier+" subscription")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 4: Add scopes to `ScopeCatalog` in `scopes.go`**

Insert before the wildcard entries (keep `api.read.*` and `*` last):

```go
	{
		Value:       "api.read.visa",
		Label:       "Read Visa & Immigration",
		Description: "Read visa routes, processing timelines, credential pathways, and language test requirements",
	},
	{
		Value:       "api.read.education",
		Label:       "Read Education",
		Description: "Read scholarships and universities",
	},
	{
		Value:       "api.read.brief",
		Label:       "Read Destination Briefs",
		Description: "Read aggregate destination briefs (premium endpoint)",
	},
	{
		Value:       "agent.use",
		Label:       "Use Agent Tools",
		Description: "Call MCP agent tools via /api/v1/mcp (read-only tool set for API keys)",
	},
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/publicapi/ -v`
Expected: PASS (new test + existing ratelimit/openapi/swagger tests)

- [ ] **Step 6: Commit**

```bash
git add internal/publicapi/tier.go internal/publicapi/tier_test.go internal/publicapi/scopes.go
git commit -m "feat(publicapi): RequireTier middleware and agent-data scopes"
```

---

### Task 3: Moat reader interfaces + `Service` delegation

**Files:**
- Create: `internal/publicapi/moat.go`
- Create: `internal/publicapi/moat_test.go`

**Interfaces:**
- Consumes: `visaroutes.Service`, `visatimeline.Service`, `credentialchecker.Service`, `languagetesthub.Service`, `scholarships.Service`, `spaces.Service`, `scamalert.Service` — all satisfy the interfaces below structurally (signatures verified against their `service.go`/`store.go` files).
- Produces: `MoatReaders` struct, `Service.SetMoatReaders`, `Service.SetCache`, `Service.SetSettingsReader`, and the read methods every later task calls.

- [ ] **Step 1: Write the failing test**

`internal/publicapi/moat_test.go`:

```go
package publicapi

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/visaroutes"
)

type stubVisaRoutes struct {
	listFn func(ctx context.Context, dest pgtype.UUID, prof, edu string, yrs, page, limit int32) ([]visaroutes.RouteDTO, int64, error)
}

func (s stubVisaRoutes) List(ctx context.Context, dest pgtype.UUID, prof, edu string, yrs, page, limit int32) ([]visaroutes.RouteDTO, int64, error) {
	return s.listFn(ctx, dest, prof, edu, yrs, page, limit)
}
func (s stubVisaRoutes) Compare(_ context.Context, _ []pgtype.UUID) ([]visaroutes.RouteDTO, error) {
	return nil, nil
}
func (s stubVisaRoutes) GetByID(_ context.Context, _ pgtype.UUID) (visaroutes.RouteDTO, error) {
	return visaroutes.RouteDTO{}, nil
}
func (s stubVisaRoutes) Enabled(_ context.Context) bool { return true }

func TestServiceListVisaRoutesDelegates(t *testing.T) {
	svc := NewService(nil)
	called := false
	svc.SetMoatReaders(&MoatReaders{
		VisaRoutes: stubVisaRoutes{listFn: func(_ context.Context, _ pgtype.UUID, _, _ string, _, page, limit int32) ([]visaroutes.RouteDTO, int64, error) {
			called = true
			if page != 2 || limit != 20 {
				t.Fatalf("page/limit = %d/%d, want 2/20", page, limit)
			}
			return []visaroutes.RouteDTO{{}}, 1, nil
		}},
	})

	routes, total, err := svc.ListVisaRoutes(context.Background(), pgtype.UUID{}, "", "", 0, 2, 20)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !called || total != 1 || len(routes) != 1 {
		t.Fatalf("delegate not invoked correctly")
	}
}

func TestServiceListVisaRoutesUnconfigured(t *testing.T) {
	svc := NewService(nil)
	_, _, err := svc.ListVisaRoutes(context.Background(), pgtype.UUID{}, "", "", 0, 1, 20)
	if err == nil {
		t.Fatal("expected error when readers not configured")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/publicapi/ -run TestServiceListVisaRoutes -v`
Expected: FAIL — `undefined: MoatReaders`, `SetMoatReaders`, `ListVisaRoutes`

- [ ] **Step 3: Implement `moat.go`**

```go
package publicapi

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/credentialchecker"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/scamalert"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/visaroutes"
	"github.com/jedafe/japa/internal/visatimeline"
)

// ErrReaderUnconfigured is returned when a moat-domain reader was not wired.
var ErrReaderUnconfigured = errors.New("publicapi: reader not configured")

// ---------------------------------------------------------------------------
// Reader interfaces — narrow slices of the domain services (agent data API).
// Ref: docs/superpowers/specs/2026-09-23-agent-data-api-design.md
// ---------------------------------------------------------------------------

// VisaRouteReader is satisfied by *visaroutes.Service.
type VisaRouteReader interface {
	List(ctx context.Context, destSpaceID pgtype.UUID, profession, education string, experienceYears, page, limit int32) ([]visaroutes.RouteDTO, int64, error)
	Compare(ctx context.Context, ids []pgtype.UUID) ([]visaroutes.RouteDTO, error)
	GetByID(ctx context.Context, id pgtype.UUID) (visaroutes.RouteDTO, error)
	Enabled(ctx context.Context) bool
}

// VisaTimelineReader is satisfied by *visatimeline.Service.
type VisaTimelineReader interface {
	GetStats(ctx context.Context, originSpaceID, destSpaceID pgtype.UUID, visaCategory, consulate string, months int) ([]visatimeline.ProcessingStatDTO, error)
	GetTrend(ctx context.Context, destSpaceID pgtype.UUID, visaCategory, consulate string, months int) ([]visatimeline.TrendPointDTO, error)
	Enabled(ctx context.Context) bool
}

// CredentialReader is satisfied by *credentialchecker.Service.
type CredentialReader interface {
	ListProfessions(ctx context.Context) ([]credentialchecker.ProfessionDTO, error)
	ListPathways(ctx context.Context, destSpaceID, issuingSpaceID, professionID pgtype.UUID, page, limit int32) ([]credentialchecker.PathwayDTO, int64, error)
	GetPathway(ctx context.Context, id pgtype.UUID) (credentialchecker.PathwayDetailDTO, error)
	Enabled(ctx context.Context) bool
}

// LanguageTestReader is satisfied by *languagetesthub.Service.
type LanguageTestReader interface {
	ListTests(ctx context.Context) ([]languagetesthub.TestDTO, error)
	GetTestBySlug(ctx context.Context, slug string) (languagetesthub.TestDTO, error)
	ListRequirements(ctx context.Context, destSpaceID pgtype.UUID, visaCategory string) ([]languagetesthub.RequirementDTO, error)
	Enabled(ctx context.Context) bool
}

// ScholarshipReader is satisfied by *scholarships.Service.
type ScholarshipReader interface {
	FindScholarships(ctx context.Context, filters scholarships.ScholarshipFilters) ([]scholarships.ScholarshipResult, int64, error)
	GetScholarshipDetail(ctx context.Context, guideID pgtype.UUID) (scholarships.ScholarshipResult, error)
	Statuses() []scholarships.Option
	ListLevels(ctx context.Context) ([]scholarships.Option, error)
	Enabled(ctx context.Context) bool
}

// SpaceReader is satisfied by *spaces.Service. Used for universities
// (university-type spaces) and destination resolution for the brief.
type SpaceReader interface {
	GetSpaceTypeBySlug(ctx context.Context, slug string) (gen.SpaceType, error)
	ListSpacesByTypePaginated(ctx context.Context, spaceTypeID pgtype.UUID, search, geoCode, region, subregion, sort string, limit, offset int32) ([]gen.ListSpacesByTypePaginatedRow, error)
	CountSpacesByType(ctx context.Context, spaceTypeID pgtype.UUID, search, geoCode, region, subregion string) (int64, error)
	GetSpaceBySlug(ctx context.Context, slug string) (gen.GetSpaceBySlugRow, error)
}

// ScamAlertReader is satisfied by *scamalert.Service (brief only in v1).
type ScamAlertReader interface {
	List(ctx context.Context, destSpaceID pgtype.UUID, scamType string, page, limit int32) ([]scamalert.ScamReportDTO, int64, error)
	Enabled(ctx context.Context) bool
}

// MoatReaders bundles every domain reader the agent data API needs.
// Injected via Service.SetMoatReaders during server wiring.
type MoatReaders struct {
	VisaRoutes    VisaRouteReader
	VisaTimeline  VisaTimelineReader
	Credentials   CredentialReader
	LanguageTests LanguageTestReader
	Scholarships  ScholarshipReader
	Spaces        SpaceReader
	ScamAlerts    ScamAlertReader
}

const universitySpaceTypeSlug = "university"

// ---------------------------------------------------------------------------
// Service wiring + delegation
// ---------------------------------------------------------------------------

// SetMoatReaders injects the domain readers used by the agent data API.
func (s *Service) SetMoatReaders(r *MoatReaders) { s.moat = r }

// SetCache injects the Valkey client used for brief response caching.
func (s *Service) SetCache(c valkeycompat.Cmdable) { s.cache = c }

// SetSettingsReader injects the settings reader used for the brief cache TTL.
func (s *Service) SetSettingsReader(sr rateLimitSettingsReader) { s.settingsReader = sr }

func (s *Service) ListVisaRoutes(ctx context.Context, destSpaceID pgtype.UUID, profession, education string, experienceYears, page, limit int32) ([]visaroutes.RouteDTO, int64, error) {
	if s.moat == nil || s.moat.VisaRoutes == nil {
		return nil, 0, ErrReaderUnconfigured
	}
	return s.moat.VisaRoutes.List(ctx, destSpaceID, profession, education, experienceYears, page, limit)
}

func (s *Service) GetVisaRoute(ctx context.Context, id pgtype.UUID) (visaroutes.RouteDTO, error) {
	if s.moat == nil || s.moat.VisaRoutes == nil {
		return visaroutes.RouteDTO{}, ErrReaderUnconfigured
	}
	return s.moat.VisaRoutes.GetByID(ctx, id)
}

func (s *Service) CompareVisaRoutes(ctx context.Context, ids []pgtype.UUID) ([]visaroutes.RouteDTO, error) {
	if s.moat == nil || s.moat.VisaRoutes == nil {
		return nil, ErrReaderUnconfigured
	}
	return s.moat.VisaRoutes.Compare(ctx, ids)
}

func (s *Service) GetVisaTimelineStats(ctx context.Context, originSpaceID, destSpaceID pgtype.UUID, visaCategory, consulate string, months int) ([]visatimeline.ProcessingStatDTO, error) {
	if s.moat == nil || s.moat.VisaTimeline == nil {
		return nil, ErrReaderUnconfigured
	}
	return s.moat.VisaTimeline.GetStats(ctx, originSpaceID, destSpaceID, visaCategory, consulate, months)
}

func (s *Service) GetVisaTimelineTrend(ctx context.Context, destSpaceID pgtype.UUID, visaCategory, consulate string, months int) ([]visatimeline.TrendPointDTO, error) {
	if s.moat == nil || s.moat.VisaTimeline == nil {
		return nil, ErrReaderUnconfigured
	}
	return s.moat.VisaTimeline.GetTrend(ctx, destSpaceID, visaCategory, consulate, months)
}

func (s *Service) ListCredentialProfessions(ctx context.Context) ([]credentialchecker.ProfessionDTO, error) {
	if s.moat == nil || s.moat.Credentials == nil {
		return nil, ErrReaderUnconfigured
	}
	return s.moat.Credentials.ListProfessions(ctx)
}

func (s *Service) ListCredentialPathways(ctx context.Context, destSpaceID, issuingSpaceID, professionID pgtype.UUID, page, limit int32) ([]credentialchecker.PathwayDTO, int64, error) {
	if s.moat == nil || s.moat.Credentials == nil {
		return nil, 0, ErrReaderUnconfigured
	}
	return s.moat.Credentials.ListPathways(ctx, destSpaceID, issuingSpaceID, professionID, page, limit)
}

func (s *Service) GetCredentialPathway(ctx context.Context, id pgtype.UUID) (credentialchecker.PathwayDetailDTO, error) {
	if s.moat == nil || s.moat.Credentials == nil {
		return credentialchecker.PathwayDetailDTO{}, ErrReaderUnconfigured
	}
	return s.moat.Credentials.GetPathway(ctx, id)
}

func (s *Service) ListLanguageTests(ctx context.Context) ([]languagetesthub.TestDTO, error) {
	if s.moat == nil || s.moat.LanguageTests == nil {
		return nil, ErrReaderUnconfigured
	}
	return s.moat.LanguageTests.ListTests(ctx)
}

func (s *Service) GetLanguageTest(ctx context.Context, slug string) (languagetesthub.TestDTO, error) {
	if s.moat == nil || s.moat.LanguageTests == nil {
		return languagetesthub.TestDTO{}, ErrReaderUnconfigured
	}
	return s.moat.LanguageTests.GetTestBySlug(ctx, slug)
}

func (s *Service) ListLanguageTestRequirements(ctx context.Context, destSpaceID pgtype.UUID, visaCategory string) ([]languagetesthub.RequirementDTO, error) {
	if s.moat == nil || s.moat.LanguageTests == nil {
		return nil, ErrReaderUnconfigured
	}
	return s.moat.LanguageTests.ListRequirements(ctx, destSpaceID, visaCategory)
}

func (s *Service) FindScholarships(ctx context.Context, filters scholarships.ScholarshipFilters) ([]scholarships.ScholarshipResult, int64, error) {
	if s.moat == nil || s.moat.Scholarships == nil {
		return nil, 0, ErrReaderUnconfigured
	}
	return s.moat.Scholarships.FindScholarships(ctx, filters)
}

func (s *Service) GetScholarship(ctx context.Context, guideID pgtype.UUID) (scholarships.ScholarshipResult, error) {
	if s.moat == nil || s.moat.Scholarships == nil {
		return scholarships.ScholarshipResult{}, ErrReaderUnconfigured
	}
	return s.moat.Scholarships.GetScholarshipDetail(ctx, guideID)
}

func (s *Service) ScholarshipStatuses() ([]scholarships.Option, error) {
	if s.moat == nil || s.moat.Scholarships == nil {
		return nil, ErrReaderUnconfigured
	}
	return s.moat.Scholarships.Statuses(), nil
}

func (s *Service) ScholarshipLevels(ctx context.Context) ([]scholarships.Option, error) {
	if s.moat == nil || s.moat.Scholarships == nil {
		return nil, ErrReaderUnconfigured
	}
	return s.moat.Scholarships.ListLevels(ctx)
}

func (s *Service) ListUniversities(ctx context.Context, search, country, sort string, limit, offset int32) ([]gen.ListSpacesByTypePaginatedRow, int64, error) {
	if s.moat == nil || s.moat.Spaces == nil {
		return nil, 0, ErrReaderUnconfigured
	}
	st, err := s.moat.Spaces.GetSpaceTypeBySlug(ctx, universitySpaceTypeSlug)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.moat.Spaces.ListSpacesByTypePaginated(ctx, st.ID, search, country, "", "", sort, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.moat.Spaces.CountSpacesByType(ctx, st.ID, search, country, "", "")
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *Service) GetUniversity(ctx context.Context, slug string) (gen.GetSpaceBySlugRow, error) {
	if s.moat == nil || s.moat.Spaces == nil {
		return gen.GetSpaceBySlugRow{}, ErrReaderUnconfigured
	}
	st, err := s.moat.Spaces.GetSpaceTypeBySlug(ctx, universitySpaceTypeSlug)
	if err != nil {
		return gen.GetSpaceBySlugRow{}, err
	}
	space, err := s.moat.Spaces.GetSpaceBySlug(ctx, slug)
	if err != nil {
		return gen.GetSpaceBySlugRow{}, err
	}
	// Public-API guard: GetSpaceBySlug is the internal reader — never leak
	// non-public/inactive spaces or non-university spaces through /public/*.
	// Verify the exact literal values against the getPublicSpaceBySlug query
	// in db/queries/spaces.sql (likely 'public' and 'active').
	if space.SpaceTypeID != st.ID || space.Visibility != "public" || space.Status != "active" {
		return gen.GetSpaceBySlugRow{}, ErrDestinationNotFound
	}
	return space, nil
}
```

Also add the fields to `Service` in `service.go` (near `tierResolver` at line ~321):

```go
	moat           *MoatReaders
	cache          valkeycompat.Cmdable
	settingsReader rateLimitSettingsReader
```

`ErrDestinationNotFound` is defined in `brief.go` (Task 6); for this task add
it temporarily or move it here — put it in `moat.go`:

```go
// ErrDestinationNotFound is returned when a slug does not resolve to an
// expected public space (used by GetUniversity and the destination brief).
var ErrDestinationNotFound = errors.New("destination_not_found")
```

**Verify the `gen.GetSpaceBySlugRow` field name** — check whether the
space-type column is `SpaceTypeID` and adjust the comparison to the actual
generated field name (read `internal/db/gen/spaces.sql.go` `GetSpaceBySlug`).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/publicapi/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/publicapi/moat.go internal/publicapi/moat_test.go internal/publicapi/service.go
git commit -m "feat(publicapi): moat reader interfaces and service delegation"
```

---

### Task 4: Visa & immigration REST handlers + route wiring

**Files:**
- Create: `internal/publicapi/handlers_moat.go`
- Create: `internal/publicapi/handlers_moat_test.go`
- Modify: `internal/server/server.go` (~line 4453, inside the `/public` group)

**Interfaces:**
- Consumes: `Service.ListVisaRoutes`, `GetVisaRoute`, `CompareVisaRoutes`, `GetVisaTimelineStats`, `GetVisaTimelineTrend`, `ListCredentialProfessions`, `ListCredentialPathways`, `GetCredentialPathway`, `ListLanguageTests`, `GetLanguageTest`, `ListLanguageTestRequirements` (Task 3); `RequireTier` (Task 2); `parsePageSize`, `parseOffset`, `parseUUIDParam`, `handleServiceError` (existing helpers).
- Produces: handler methods referenced by route registrations in this task.

- [ ] **Step 1: Write the failing test**

`internal/publicapi/handlers_moat_test.go`:

```go
package publicapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/visaroutes"
)

func withAPIKey(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), apiKeyContextKey,
		&APIKeyContext{UserID: pgtype.UUID{Valid: true}, Scopes: []string{"api.read.visa"}}))
}

func TestListVisaRoutesHandler(t *testing.T) {
	svc := NewService(nil)
	svc.SetMoatReaders(&MoatReaders{
		VisaRoutes: stubVisaRoutes{listFn: func(_ context.Context, _ pgtype.UUID, _, _ string, _, _, _ int32) ([]visaroutes.RouteDTO, int64, error) {
			return []visaroutes.RouteDTO{{}, {}}, 2, nil
		}},
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	h.ListVisaRoutes(rec, withAPIKey(httptest.NewRequest(http.MethodGet, "/public/visa-routes", nil)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestListVisaRoutesHandlerNoKey(t *testing.T) {
	h := NewHandlers(NewService(nil))
	rec := httptest.NewRecorder()
	h.ListVisaRoutes(rec, httptest.NewRequest(http.MethodGet, "/public/visa-routes", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/publicapi/ -run TestListVisaRoutes -v`
Expected: FAIL — `h.ListVisaRoutes undefined`

- [ ] **Step 3: Implement `handlers_moat.go`**

```go
package publicapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// requireAPIKey writes a 401 and returns false when no key context exists.
func requireAPIKey(w http.ResponseWriter, r *http.Request) bool {
	if APIKeyFromContext(r.Context()) == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return false
	}
	return true
}

func parseUUIDQuery(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if v := r.URL.Query().Get(param); v != "" {
		if err := id.Scan(v); err != nil {
			return pgtype.UUID{}, err
		}
	}
	return id, nil
}

// ListVisaRoutes handles GET /public/visa-routes.
// Params: dest, profession, education, experience_years, page_size, offset.
func (h *Handlers) ListVisaRoutes(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	dest, err := parseUUIDQuery(r, "dest")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dest space ID")
		return
	}
	years, _ := strconv.ParseInt(r.URL.Query().Get("experience_years"), 10, 32)
	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)
	page := offset/limit + 1

	routes, total, err := h.Service.ListVisaRoutes(r.Context(), dest,
		r.URL.Query().Get("profession"), r.URL.Query().Get("education"),
		int32(years), page, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": routes, "total": total, "limit": limit, "offset": offset,
	})
}

// GetVisaRoute handles GET /public/visa-routes/{id}.
func (h *Handlers) GetVisaRoute(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid visa route ID")
		return
	}
	route, err := h.Service.GetVisaRoute(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": route})
}

// CompareVisaRoutes handles GET /public/visa-routes/compare?ids=a,b,c.
func (h *Handlers) CompareVisaRoutes(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	raw := r.URL.Query().Get("ids")
	if raw == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_ids", "ids query param is required")
		return
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 5 {
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_ids", "At most 5 route IDs may be compared")
		return
	}
	ids := make([]pgtype.UUID, 0, len(parts))
	for _, p := range parts {
		var id pgtype.UUID
		if err := id.Scan(strings.TrimSpace(p)); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid route ID in ids list")
			return
		}
		ids = append(ids, id)
	}
	routes, err := h.Service.CompareVisaRoutes(r.Context(), ids)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": routes})
}

// GetVisaTimelineStats handles GET /public/visa-timelines/stats.
// Params: origin, dest, visa, consulate, months.
func (h *Handlers) GetVisaTimelineStats(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	origin, err := parseUUIDQuery(r, "origin")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid origin space ID")
		return
	}
	dest, err := parseUUIDQuery(r, "dest")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dest space ID")
		return
	}
	months := 0
	if v := r.URL.Query().Get("months"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 60 {
			months = n
		}
	}
	stats, err := h.Service.GetVisaTimelineStats(r.Context(), origin, dest,
		r.URL.Query().Get("visa"), r.URL.Query().Get("consulate"), months)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": stats})
}

// GetVisaTimelineTrend handles GET /public/visa-timelines/stats/trend.
func (h *Handlers) GetVisaTimelineTrend(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	dest, err := parseUUIDQuery(r, "dest")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dest space ID")
		return
	}
	months := 12
	if v := r.URL.Query().Get("months"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 60 {
			months = n
		}
	}
	trend, err := h.Service.GetVisaTimelineTrend(r.Context(), dest,
		r.URL.Query().Get("visa"), r.URL.Query().Get("consulate"), months)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": trend})
}

// ListCredentialProfessions handles GET /public/credentials/professions.
func (h *Handlers) ListCredentialProfessions(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	profs, err := h.Service.ListCredentialProfessions(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profs})
}

// ListCredentialPathways handles GET /public/credentials.
// Params: dest, issuing, profession, page_size, offset.
func (h *Handlers) ListCredentialPathways(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	dest, err := parseUUIDQuery(r, "dest")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dest space ID")
		return
	}
	issuing, err := parseUUIDQuery(r, "issuing")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid issuing space ID")
		return
	}
	profession, err := parseUUIDQuery(r, "profession")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profession ID")
		return
	}
	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)
	page := offset/limit + 1

	pathways, total, err := h.Service.ListCredentialPathways(r.Context(), dest, issuing, profession, page, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": pathways, "total": total, "limit": limit, "offset": offset,
	})
}

// GetCredentialPathway handles GET /public/credentials/{id}.
func (h *Handlers) GetCredentialPathway(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid pathway ID")
		return
	}
	p, err := h.Service.GetCredentialPathway(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": p})
}

// ListLanguageTests handles GET /public/language-tests.
func (h *Handlers) ListLanguageTests(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	tests, err := h.Service.ListLanguageTests(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tests})
}

// GetLanguageTest handles GET /public/language-tests/{slug}.
func (h *Handlers) GetLanguageTest(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}
	test, err := h.Service.GetLanguageTest(r.Context(), slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": test})
}

// ListLanguageTestRequirements handles GET /public/language-tests/requirements.
// Params: dest (required), visa.
func (h *Handlers) ListLanguageTestRequirements(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	dest, err := parseUUIDQuery(r, "dest")
	if err != nil || !dest.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "A valid dest space ID is required")
		return
	}
	reqs, err := h.Service.ListLanguageTestRequirements(r.Context(), dest, r.URL.Query().Get("visa"))
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": reqs})
}
```

Note: chi registers `/public/visa-routes/compare` as a static segment which
takes precedence over `{id}`, and likewise `/public/language-tests/requirements`
over `{slug}` — register static routes FIRST in server.go for clarity anyway.

- [ ] **Step 4: Wire routes in `internal/server/server.go`**

Inside the existing `r.Group` that already has `APIKeyMiddleware` +
`SlidingWindowRateLimitWithSettings` (after the trending routes, ~line 4453):

```go
			// Agent Data API — visa & immigration (spec 2026-09-23-agent-data-api)
			r.With(features.Gate(featureChecker, "visa_routes.enabled"), publicapi.RequireScope("api.read.visa"), publicapi.RequireTier(publicAPIService, "pro")).
				Get("/public/visa-routes/compare", publicAPIHandlers.CompareVisaRoutes)
			r.With(features.Gate(featureChecker, "visa_routes.enabled"), publicapi.RequireScope("api.read.visa")).
				Get("/public/visa-routes", publicAPIHandlers.ListVisaRoutes)
			r.With(features.Gate(featureChecker, "visa_routes.enabled"), publicapi.RequireScope("api.read.visa")).
				Get("/public/visa-routes/{id}", publicAPIHandlers.GetVisaRoute)

			r.With(features.Gate(featureChecker, "visa_timeline.enabled"), publicapi.RequireScope("api.read.visa")).
				Get("/public/visa-timelines/stats", publicAPIHandlers.GetVisaTimelineStats)
			r.With(features.Gate(featureChecker, "visa_timeline.enabled"), publicapi.RequireScope("api.read.visa"), publicapi.RequireTier(publicAPIService, "pro")).
				Get("/public/visa-timelines/stats/trend", publicAPIHandlers.GetVisaTimelineTrend)

			r.With(features.Gate(featureChecker, "credential_checker.enabled"), publicapi.RequireScope("api.read.visa")).
				Get("/public/credentials/professions", publicAPIHandlers.ListCredentialProfessions)
			r.With(features.Gate(featureChecker, "credential_checker.enabled"), publicapi.RequireScope("api.read.visa")).
				Get("/public/credentials", publicAPIHandlers.ListCredentialPathways)
			r.With(features.Gate(featureChecker, "credential_checker.enabled"), publicapi.RequireScope("api.read.visa")).
				Get("/public/credentials/{id}", publicAPIHandlers.GetCredentialPathway)

			r.With(features.Gate(featureChecker, "language_test_hub.enabled"), publicapi.RequireScope("api.read.visa")).
				Get("/public/language-tests/requirements", publicAPIHandlers.ListLanguageTestRequirements)
			r.With(features.Gate(featureChecker, "language_test_hub.enabled"), publicapi.RequireScope("api.read.visa")).
				Get("/public/language-tests", publicAPIHandlers.ListLanguageTests)
			r.With(features.Gate(featureChecker, "language_test_hub.enabled"), publicapi.RequireScope("api.read.visa")).
				Get("/public/language-tests/{slug}", publicAPIHandlers.GetLanguageTest)
```

And earlier in the function (near `publicAPIHandlers` construction, ~line 2315),
inject the readers — this line goes where the domain services are already in
scope:

```go
	publicAPIService.SetMoatReaders(&publicapi.MoatReaders{
		VisaRoutes:    visaRoutesService,
		VisaTimeline:  visaTimelineService,
		Credentials:   credentialCheckerService,
		LanguageTests: languageTestHubService,
		Scholarships:  scholarshipsService,
		Spaces:        spaceService,
		ScamAlerts:    scamAlertService,
	})
```

Check the concrete types: `visaRoutesService` is `*visaroutes.Service` etc.
If any constructor returns a different type that doesn't satisfy the
interface, write a 5-line adapter in `server.go`'s helpers rather than
widening the interface. `spaceService` must satisfy `SpaceReader` — verify
its methods match (it is the same `*spaces.Service` universities uses).

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/publicapi/ -v && go build ./internal/server/`
Expected: PASS, build clean

- [ ] **Step 6: Commit**

```bash
git add internal/publicapi/handlers_moat.go internal/publicapi/handlers_moat_test.go internal/server/server.go
git commit -m "feat(publicapi): visa & immigration agent data endpoints"
```

---

### Task 5: Education REST handlers + route wiring

**Files:**
- Modify: `internal/publicapi/handlers_moat.go` (append education handlers)
- Modify: `internal/publicapi/handlers_moat_test.go` (append tests)
- Modify: `internal/server/server.go` (same group as Task 4)

**Interfaces:**
- Consumes: `Service.FindScholarships`, `GetScholarship`, `ScholarshipStatuses`, `ScholarshipLevels`, `ListUniversities`, `GetUniversity` (Task 3). `scholarships.ScholarshipFilters` — **read `internal/scholarships/service.go` around line 651 and the web handler's filter parsing (`internal/scholarships/handlers.go` `HandleFindScholarships`) and mirror its query-param names exactly** so the public and web surfaces accept identical filters.

- [ ] **Step 1: Write the failing test**

Append to `handlers_moat_test.go`:

```go
type stubScholarships struct {
	findFn func(ctx context.Context, f scholarships.ScholarshipFilters) ([]scholarships.ScholarshipResult, int64, error)
}

func (s stubScholarships) FindScholarships(ctx context.Context, f scholarships.ScholarshipFilters) ([]scholarships.ScholarshipResult, int64, error) {
	return s.findFn(ctx, f)
}
func (s stubScholarships) GetScholarshipDetail(_ context.Context, _ pgtype.UUID) (scholarships.ScholarshipResult, error) {
	return scholarships.ScholarshipResult{}, nil
}
func (s stubScholarships) Statuses() []scholarships.Option                 { return nil }
func (s stubScholarships) ListLevels(_ context.Context) ([]scholarships.Option, error) {
	return nil, nil
}
func (s stubScholarships) Enabled(_ context.Context) bool { return true }

func TestFindScholarshipsHandler(t *testing.T) {
	svc := NewService(nil)
	svc.SetMoatReaders(&MoatReaders{
		Scholarships: stubScholarships{findFn: func(_ context.Context, _ scholarships.ScholarshipFilters) ([]scholarships.ScholarshipResult, int64, error) {
			return []scholarships.ScholarshipResult{{}}, 1, nil
		}},
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	h.FindScholarships(rec, withAPIKey(httptest.NewRequest(http.MethodGet, "/public/scholarships?level=masters", nil)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}
```

(add `"github.com/jedafe/japa/internal/scholarships"` to imports)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/publicapi/ -run TestFindScholarships -v`
Expected: FAIL — `h.FindScholarships undefined`

- [ ] **Step 3: Implement the education handlers**

Append to `handlers_moat.go`:

```go
// FindScholarships handles GET /public/scholarships.
// Filter params mirror the web handler (HandleFindScholarships) exactly.
func (h *Handlers) FindScholarships(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	q := r.URL.Query()
	filters := scholarships.ScholarshipFilters{
		Country:      q.Get("country"),
		Level:        q.Get("level"),
		Q:            q.Get("q"),
		Sort:         q.Get("sort"),
		DeadlineFrom: q.Get("deadline_from"),
		DeadlineTo:   q.Get("deadline_to"),
	}
	if v := q.Get("include_expired"); v != "" {
		filters.IncludeExpired, _ = strconv.ParseBool(v)
	}
	if v := q.Get("min_amount"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			filters.MinAmount = f
		}
	}
	if v := q.Get("max_amount"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			filters.MaxAmount = f
		}
	}
	if v := q.Get("renewable"); v != "" {
		b, _ := strconv.ParseBool(v)
		filters.Renewable = &b
	}
	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)
	filters.Page = int(offset/limit + 1)
	filters.Limit = int(limit)

	results, total, err := h.Service.FindScholarships(r.Context(), filters)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": results, "total": total, "limit": limit, "offset": offset,
	})
}

// GetScholarship handles GET /public/scholarships/{id} — id is the guide UUID.
func (h *Handlers) GetScholarship(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid scholarship ID")
		return
	}
	s, err := h.Service.GetScholarship(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": s})
}

// ListScholarshipStatuses handles GET /public/scholarships/statuses.
func (h *Handlers) ListScholarshipStatuses(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	statuses, err := h.Service.ScholarshipStatuses()
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": statuses})
}

// ListScholarshipLevels handles GET /public/scholarships/levels.
func (h *Handlers) ListScholarshipLevels(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	levels, err := h.Service.ScholarshipLevels(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": levels})
}

// ListUniversities handles GET /public/universities.
// Params: q, country, sort, page_size, offset.
func (h *Handlers) ListUniversities(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)
	items, total, err := h.Service.ListUniversities(r.Context(),
		strings.TrimSpace(r.URL.Query().Get("q")),
		strings.TrimSpace(r.URL.Query().Get("country")),
		strings.TrimSpace(r.URL.Query().Get("sort")),
		limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items, "total": total, "limit": limit, "offset": offset,
	})
}

// GetUniversity handles GET /public/universities/{slug}.
func (h *Handlers) GetUniversity(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}
	uni, err := h.Service.GetUniversity(r.Context(), slug)
	if err != nil {
		if errors.Is(err, ErrDestinationNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "University not found")
			return
		}
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": uni})
}
```

(add `"errors"`, `"strconv"`, and `"github.com/jedafe/japa/internal/scholarships"`
to the `handlers_moat.go` imports — filter parsing mirrors
`internal/scholarships/handlers.go` `HandleFindScholarships`.)

- [ ] **Step 4: Wire routes**

Append to the same group in server.go:

```go
			// Agent Data API — education
			r.With(features.Gate(featureChecker, "scholarship_finder.enabled"), publicapi.RequireScope("api.read.education")).
				Get("/public/scholarships/statuses", publicAPIHandlers.ListScholarshipStatuses)
			r.With(features.Gate(featureChecker, "scholarship_finder.enabled"), publicapi.RequireScope("api.read.education")).
				Get("/public/scholarships/levels", publicAPIHandlers.ListScholarshipLevels)
			r.With(features.Gate(featureChecker, "scholarship_finder.enabled"), publicapi.RequireScope("api.read.education")).
				Get("/public/scholarships", publicAPIHandlers.FindScholarships)
			r.With(features.Gate(featureChecker, "scholarship_finder.enabled"), publicapi.RequireScope("api.read.education")).
				Get("/public/scholarships/{id}", publicAPIHandlers.GetScholarship)

			r.With(publicapi.RequireScope("api.read.education")).
				Get("/public/universities", publicAPIHandlers.ListUniversities)
			r.With(publicapi.RequireScope("api.read.education")).
				Get("/public/universities/{slug}", publicAPIHandlers.GetUniversity)
```

(universities have no feature flag — they are plain spaces.)

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/publicapi/ -v && go build ./internal/server/`
Expected: PASS, build clean

- [ ] **Step 6: Commit**

```bash
git add internal/publicapi/handlers_moat.go internal/publicapi/handlers_moat_test.go internal/server/server.go
git commit -m "feat(publicapi): education agent data endpoints"
```

---

### Task 6: Destination brief endpoint

**Files:**
- Create: `internal/publicapi/brief.go`
- Create: `internal/publicapi/brief_test.go`
- Modify: `internal/server/server.go` (route + `SetCache`/`SetSettingsReader` wiring)

**Interfaces:**
- Consumes: all `MoatReaders` (Task 3), `RequireTier` (Task 2), Valkey key `jc:api_dest_brief:{slug}:{include_hash}` (cataloged Task 1), setting `api.destination_brief.cache_ttl_seconds` (cataloged Task 1).
- Produces: `DestinationBrief` type, `Service.GetDestinationBrief(ctx, slug string, include []string)`, `Handlers.GetDestinationBrief` — also consumed by `destination.brief` MCP tool in Task 8.

- [ ] **Step 1: Write the failing test**

`internal/publicapi/brief_test.go`:

```go
package publicapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/visaroutes"
)

// stubSpaceReader implements SpaceReader; only the methods a test needs
// are populated.
type stubSpaceReader struct {
	getBySlugFn func(ctx context.Context, slug string) (gen.GetSpaceBySlugRow, error)
}

func (s stubSpaceReader) GetSpaceTypeBySlug(_ context.Context, _ string) (gen.SpaceType, error) {
	return gen.SpaceType{}, nil
}
func (s stubSpaceReader) ListSpacesByTypePaginated(_ context.Context, _ pgtype.UUID, _, _, _, _, _ string, _, _ int32) ([]gen.ListSpacesByTypePaginatedRow, error) {
	return nil, nil
}
func (s stubSpaceReader) CountSpacesByType(_ context.Context, _ pgtype.UUID, _, _, _, _ string) (int64, error) {
	return 0, nil
}
func (s stubSpaceReader) GetSpaceBySlug(ctx context.Context, slug string) (gen.GetSpaceBySlugRow, error) {
	return s.getBySlugFn(ctx, slug)
}

func TestGetDestinationBriefUnknownSlug(t *testing.T) {
	svc := NewService(nil)
	svc.SetMoatReaders(&MoatReaders{
		Spaces: stubSpaceReader{getBySlugFn: func(_ context.Context, _ string) (gen.GetSpaceBySlugRow, error) {
			return gen.GetSpaceBySlugRow{}, pgx.ErrNoRows
		}},
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/public/destinations/nowhere/brief", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", "nowhere")
	r = withAPIKey(r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx)))
	h.GetDestinationBrief(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDestinationBriefPartialFailure(t *testing.T) {
	svc := NewService(nil)
	svc.SetMoatReaders(&MoatReaders{
		Spaces: stubSpaceReader{getBySlugFn: func(_ context.Context, _ string) (gen.GetSpaceBySlugRow, error) {
			return gen.GetSpaceBySlugRow{Visibility: "public", Status: "active", Name: "Canada"}, nil
		}},
		VisaRoutes: stubVisaRoutes{listFn: func(_ context.Context, _ pgtype.UUID, _, _ string, _, _, _ int32) ([]visaroutes.RouteDTO, int64, error) {
			return nil, 0, errors.New("boom")
		}},
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/public/destinations/canada/brief?include=visa", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", "canada")
	r = withAPIKey(r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx)))
	h.GetDestinationBrief(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"visa"`) ||
		!strings.Contains(rec.Body.String(), "sections_unavailable") {
		t.Fatalf("expected visa in sections_unavailable; body: %s", rec.Body.String())
	}
}
```

(add `"github.com/go-chi/chi/v5"` to test imports — chi route context is
built manually because the handler is invoked without a router.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/publicapi/ -run TestGetDestinationBrief -v`
Expected: FAIL — `h.GetDestinationBrief undefined`

- [ ] **Step 3: Implement `brief.go`**

```go
package publicapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visaroutes"
	"golang.org/x/sync/errgroup"
)

// briefSections enumerates the composable brief sections (the `include`
// query param values).
var briefSections = []string{"visa", "timelines", "universities", "scholarships", "scam_alerts"}

// DestinationBrief is the aggregate document returned by
// GET /public/destinations/{slug}/brief.
type DestinationBrief struct {
	Destination         gen.GetSpaceBySlugRow `json:"destination"`
	Summary             string                `json:"summary"`
	Sections            map[string]any        `json:"sections"`
	SectionsUnavailable []string              `json:"sections_unavailable"`
	GeneratedAt         time.Time             `json:"generated_at"`
}

// GetDestinationBrief resolves the destination space then composes the
// requested sections in parallel. A section whose reader is disabled or
// errors is omitted and named in SectionsUnavailable — the brief is
// best-effort, only an unresolvable destination is a hard error.
func (s *Service) GetDestinationBrief(ctx context.Context, slug string, include []string) (*DestinationBrief, error) {
	if s.moat == nil || s.moat.Spaces == nil {
		return nil, ErrReaderUnconfigured
	}

	if len(include) == 0 {
		include = briefSections
	}
	include = dedupeStrings(include)

	cacheKey := valkey.MustKey("api_dest_brief:%s:%s", slug, briefIncludeHash(include))
	if s.cache != nil {
		if raw, err := s.cache.Get(ctx, cacheKey).Bytes(); err == nil {
			var cached DestinationBrief
			if json.Unmarshal(raw, &cached) == nil {
				return &cached, nil
			}
		}
	}

	dest, err := s.moat.Spaces.GetSpaceBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDestinationNotFound
		}
		return nil, err
	}
	// Public-API guard — same literal values as GetUniversity.
	if dest.Visibility != "public" || dest.Status != "active" {
		return nil, ErrDestinationNotFound
	}

	brief := &DestinationBrief{
		Destination: dest,
		Sections:    map[string]any{},
		GeneratedAt: time.Now().UTC(),
	}

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)

	wants := func(name string) bool {
		for _, i := range include {
			if i == name {
				return true
			}
		}
		return false
	}
	fail := func(name string) {
		mu.Lock()
		brief.SectionsUnavailable = append(brief.SectionsUnavailable, name)
		mu.Unlock()
	}
	put := func(name string, v any) {
		mu.Lock()
		brief.Sections[name] = v
		mu.Unlock()
	}

	if wants("visa") {
		g.Go(func() error {
			if s.moat.VisaRoutes == nil || !s.moat.VisaRoutes.Enabled(gctx) {
				fail("visa")
				return nil
			}
			routes, _, err := s.moat.VisaRoutes.List(gctx, dest.ID, "", "", 0, 1, 10)
			if err != nil {
				fail("visa")
				return nil
			}
			put("visa", map[string]any{"routes": routes})
			return nil
		})
	}
	if wants("timelines") {
		g.Go(func() error {
			if s.moat.VisaTimeline == nil || !s.moat.VisaTimeline.Enabled(gctx) {
				fail("timelines")
				return nil
			}
			stats, err := s.moat.VisaTimeline.GetStats(gctx, pgtype.UUID{}, dest.ID, "", "", 0)
			if err != nil {
				fail("timelines")
				return nil
			}
			put("timelines", map[string]any{"stats": stats})
			return nil
		})
	}
	if wants("universities") {
		g.Go(func() error {
			items, total, err := s.ListUniversities(gctx, "", "", "", 10, 0)
			if err != nil {
				fail("universities")
				return nil
			}
			put("universities", map[string]any{"items": items, "total": total})
			return nil
		})
	}
	if wants("scholarships") {
		g.Go(func() error {
			if s.moat.Scholarships == nil || !s.moat.Scholarships.Enabled(gctx) {
				fail("scholarships")
				return nil
			}
			country := ""
			if dest.GeoCode != nil {
				country = *dest.GeoCode
			}
			results, total, err := s.moat.Scholarships.FindScholarships(gctx,
				scholarships.ScholarshipFilters{Country: country, Limit: 10, Page: 1})
			if err != nil {
				fail("scholarships")
				return nil
			}
			put("scholarships", map[string]any{"items": results, "total": total})
			return nil
		})
	}
	if wants("scam_alerts") {
		g.Go(func() error {
			if s.moat.ScamAlerts == nil || !s.moat.ScamAlerts.Enabled(gctx) {
				fail("scam_alerts")
				return nil
			}
			alerts, _, err := s.moat.ScamAlerts.List(gctx, dest.ID, "", 1, 5)
			if err != nil {
				fail("scam_alerts")
				return nil
			}
			put("scam_alerts", map[string]any{"items": alerts})
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err // unreachable — goroutines never return non-nil
	}

	sort.Strings(brief.SectionsUnavailable)
	brief.Summary = renderBriefSummary(dest, brief)

	if s.cache != nil {
		ttl := 900
		if s.settingsReader != nil {
			ttl = s.settingsReader.GetIntOr(ctx, "api.destination_brief.cache_ttl_seconds", 900)
		}
		if raw, err := json.Marshal(brief); err == nil {
			_ = s.cache.Set(ctx, cacheKey, raw, time.Duration(ttl)*time.Second).Err()
		}
	}

	return brief, nil
}

// renderBriefSummary produces a deterministic markdown overview — no LLM.
func renderBriefSummary(dest gen.GetSpaceBySlugRow, b *DestinationBrief) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s — destination brief\n\n", dest.Name)
	if v, ok := b.Sections["visa"].(map[string]any); ok {
		fmt.Fprintf(&sb, "- **Visa routes:** %d top routes included\n", len(v["routes"].([]visaroutes.RouteDTO)))
	}
	if len(b.SectionsUnavailable) > 0 {
		fmt.Fprintf(&sb, "- _Sections unavailable: %s_\n", strings.Join(b.SectionsUnavailable, ", "))
	}
	return sb.String()
}

func briefIncludeHash(include []string) string {
	sorted := append([]string(nil), include...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, ",")))
	return hex.EncodeToString(sum[:])[:12]
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// GetDestinationBrief handles GET /public/destinations/{slug}/brief.
func (h *Handlers) GetDestinationBrief(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}
	var include []string
	if v := r.URL.Query().Get("include"); v != "" {
		include = strings.Split(v, ",")
	}
	brief, err := h.Service.GetDestinationBrief(r.Context(), slug, include)
	if err != nil {
		if errors.Is(err, ErrDestinationNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "destination_not_found", "Destination not found")
			return
		}
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": brief})
}
```

`briefSections`, `DestinationBrief`, `GetDestinationBrief` (service method),
`renderBriefSummary`, `briefIncludeHash`, `dedupeStrings`, and the
`Handlers.GetDestinationBrief` handler all live in `brief.go`.

- [ ] **Step 4: Wire route + deps in `server.go`**

Inside the public group:

```go
			// Agent Data API — destination brief (premium)
			r.With(publicapi.RequireScope("api.read.brief"), publicapi.RequireTier(publicAPIService, "pro")).
				Get("/public/destinations/{slug}/brief", publicAPIHandlers.GetDestinationBrief)
```

With the other `publicAPIService` setters (~line 2346):

```go
	publicAPIService.SetCache(vkClient.Redis())
	publicAPIService.SetSettingsReader(settingsService)
```

Verify `settingsService` satisfies `rateLimitSettingsReader` (`GetIntOr`) — it
is the same object already passed to `SlidingWindowRateLimitWithSettings`.

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/publicapi/ -v && go build ./internal/server/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/publicapi/brief.go internal/publicapi/brief_test.go internal/server/server.go
git commit -m "feat(publicapi): aggregate destination brief endpoint"
```

---

### Task 7: MCP API-key hardening

**Files:**
- Modify: `internal/publicapi/types.go` (export `WithAPIKeyContext`)
- Modify: `internal/mcp/middleware.go` (store key ctx + API-key-caller marker)
- Modify: `internal/mcp/mcp.go` (read-only allowlist)
- Modify: `internal/mcp/mcp_test.go` (new tests)
- Modify: `internal/server/server.go` (~line 4786, add rate limiter to `/mcp`)

**Interfaces:**
- Produces: `publicapi.WithAPIKeyContext(ctx, *APIKeyContext) context.Context`
- Consumes: `publicapi.SlidingWindowRateLimitWithSettings` (existing)

- [ ] **Step 1: Write the failing tests**

Append to `internal/mcp/mcp_test.go`:

```go
func TestAPIKeyCallerGetsReadOnlyCatalog(t *testing.T) {
	srv, deps := newTestMCP(t)
	_ = deps

	// register a write tool to prove it is filtered
	deps.registry.RegisterConfig("content.create", ai.ToolConfig{FunctionName: "content.create"}, "", pgtype.UUID{})
	deps.registry.RegisterConfig("content.read", ai.ToolConfig{FunctionName: "content.read"}, "", pgtype.UUID{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.AuthenticatedUser{UserID: testUserID})
	ctx = context.WithValue(ctx, apiKeyCallerKey, true)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req.WithContext(ctx))

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	tools := resp["result"].(map[string]any)["tools"].([]any)
	for _, tool := range tools {
		name := tool.(map[string]any)["name"].(string)
		if name == "content.create" {
			t.Fatal("write tool leaked to API-key caller")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run TestAPIKeyCaller -v`
Expected: FAIL — `undefined: apiKeyCallerKey` (or write tool leaks)

- [ ] **Step 3: Implement**

`internal/publicapi/types.go` — append:

```go
// WithAPIKeyContext stores an APIKeyContext in ctx. Used by the MCP auth
// middleware so the public API rate limiter sees API-key callers.
func WithAPIKeyContext(ctx context.Context, k *APIKeyContext) context.Context {
	return context.WithValue(ctx, apiKeyContextKey, k)
}
```

`internal/mcp/middleware.go` — in `AuthMiddleware`'s apiKey branch, replace
the final context build (lines ~31-48) with:

```go
			ctx := r.Context()
			if apiKey != nil {
				if !publicapi.HasScope(apiKey.Scopes, agentUsePermission) {
					http.Error(w, `{"error":{"code":"forbidden","message":"API key lacks required scope: agent.use"}}`, http.StatusForbidden)
					return
				}
				// Expose the key to the public API rate limiter and mark the
				// caller so tools/list + tools/call restrict to read-only.
				ctx = publicapi.WithAPIKeyContext(ctx, apiKey)
				ctx = context.WithValue(ctx, apiKeyCallerKey, true)
			} else {
				if !resolver.HasPermission(r.Context(), user.UserID, agentUsePermission) {
					http.Error(w, `{"error":{"code":"forbidden","message":"You do not have permission to use the agent"}}`, http.StatusForbidden)
					return
				}
			}

			ctx = context.WithValue(ctx, auth.UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
```

Add near the top of `mcp.go`:

```go
type mcpContextKey struct{ name string }

var apiKeyCallerKey = mcpContextKey{"api_key_caller"}

// isAPIKeyCaller reports whether the request was authenticated with an API
// key (vs a user JWT). API-key callers get a read-only tool catalog.
func isAPIKeyCaller(ctx context.Context) bool {
	v, _ := ctx.Value(apiKeyCallerKey).(bool)
	return v
}

// apiKeyToolAllowlist is the read-only tool set exposed to API-key callers.
// JWT-authenticated callers keep their full RBAC-filtered catalog.
var apiKeyToolAllowlist = map[string]bool{
	"content.read":       true,
	"content.search":     true,
	"rag.search":         true,
	"spaces.find":        true,
	"spaces.read":        true,
	"spaces.list":        true,
	"scholarship.search": true,
	"visa.search":        true,
	"visa.get":           true,
	"visa.compare":       true,
	"university.get":     true,
	"destination.brief":  true,
}
```

In `handleToolsList`, after building `tools`, filter when API-key caller:

```go
	if isAPIKeyCaller(ctx) {
		filtered := tools[:0]
		for _, t := range tools {
			if apiKeyToolAllowlist[t.Name] {
				filtered = append(filtered, t)
			}
		}
		tools = filtered
	}
```

In `handleToolsCall`, replace the `toolInCatalog` check with:

```go
	if isAPIKeyCaller(ctx) && !apiKeyToolAllowlist[callReq.Name] {
		return s.errorResp(req.ID, -32602, "Invalid params", fmt.Sprintf("tool %q is not available to API-key callers", callReq.Name)), false
	}
	if !s.toolInCatalog(ctx, user, callReq.Name) {
		return s.errorResp(req.ID, -32602, "Invalid params", fmt.Sprintf("tool %q is not available", callReq.Name)), false
	}
```

`internal/server/server.go` line ~4786 — add the rate limiter:

```go
			r.With(mcp.AuthMiddleware(kr, vkClient.Redis(), publicAPIService, rbacResolver),
				publicapi.SlidingWindowRateLimitWithSettings(vkClient.Redis(), settingsService, publicAPIService)).
				Handle("/mcp", mcpServer)
```

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/mcp/ ./internal/publicapi/ -v && go build ./internal/server/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/publicapi/types.go internal/mcp/ internal/server/server.go
git commit -m "fix(mcp): rate-limit API-key callers and restrict to read-only tools"
```

---

### Task 8: Moat MCP tools + ai_tools seed migration

**Files:**
- Create: `internal/ai/tools_moat.go`
- Modify: `internal/ai/tools_deps.go` (add `PublicAPI` field)
- Modify: `internal/ai/tools_register.go` (register new handlers in `RegisterPhase9Tools` or a new `RegisterAgentDataTools` called from server.go)
- Modify: `internal/server/server.go` (set `agentDeps.PublicAPI = publicAPIService`; call the register function if new)
- Create: `migrations/000722_seed_ai_tools_agent_data.up.sql` + `.down.sql`

**Interfaces:**
- Consumes: `publicapi.Service` moat methods (Tasks 3–6) via a narrow `PublicAPIReader` interface on `ToolDeps`.
- Produces: tool handlers `VisaSearchHandler`, `VisaGetHandler`, `VisaCompareHandler`, `UniversityGetHandler`, `DestinationBriefHandler` registered under the function names in the allowlist (Task 7).

- [ ] **Step 1: Write the failing test**

`internal/ai/tools_moat_test.go`:

```go
package ai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type stubPublicAPI struct {
	listVisaFn func(ctx context.Context, dest pgtype.UUID, prof, edu string, yrs, page, limit int32) ([]visaroutes.RouteDTO, int64, error)
}

func (s stubPublicAPI) ListVisaRoutes(ctx context.Context, dest pgtype.UUID, prof, edu string, yrs, page, limit int32) ([]visaroutes.RouteDTO, int64, error) {
	return s.listVisaFn(ctx, dest, prof, edu, yrs, page, limit)
}
// ...remaining PublicAPIReader methods as no-ops...

func TestVisaSearchTool(t *testing.T) {
	h := VisaSearchHandler(&ToolDeps{PublicAPI: stubPublicAPI{
		listVisaFn: func(_ context.Context, _ pgtype.UUID, _, _ string, _, _, _ int32) ([]visaroutes.RouteDTO, int64, error) {
			return []visaroutes.RouteDTO{{}}, 1, nil
		},
	}})
	res, err := h(context.Background(), json.RawMessage(`{"destination_space_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil || res == nil || res.Status != ToolCallStatusSuccess {
		t.Fatalf("res=%v err=%v", res, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/ -run TestVisaSearchTool -v`
Expected: FAIL — `undefined: VisaSearchHandler`, `ToolDeps.PublicAPI`

- [ ] **Step 3: Implement `tools_moat.go` + ToolDeps field**

`internal/ai/tools_deps.go` — add to `ToolDeps`:

```go
	// PublicAPI backs the agent data tools (spec 2026-09-23-agent-data-api).
	PublicAPI PublicAPIReader
```

`internal/ai/tools_moat.go`:

```go
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/credentialchecker"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/visaroutes"
	"github.com/jedafe/japa/internal/visatimeline"
)

// PublicAPIReader is the slice of *publicapi.Service the agent data tools
// need. Satisfied directly by publicapi.Service — same methods the REST
// endpoints call, so MCP and REST can never drift.
type PublicAPIReader interface {
	ListVisaRoutes(ctx context.Context, dest pgtype.UUID, prof, edu string, yrs, page, limit int32) ([]visaroutes.RouteDTO, int64, error)
	GetVisaRoute(ctx context.Context, id pgtype.UUID) (visaroutes.RouteDTO, error)
	CompareVisaRoutes(ctx context.Context, ids []pgtype.UUID) ([]visaroutes.RouteDTO, error)
	GetVisaTimelineStats(ctx context.Context, origin, dest pgtype.UUID, cat, consulate string, months int) ([]visatimeline.ProcessingStatDTO, error)
	ListCredentialPathways(ctx context.Context, dest, issuing, prof pgtype.UUID, page, limit int32) ([]credentialchecker.PathwayDTO, int64, error)
	ListLanguageTests(ctx context.Context) ([]languagetesthub.TestDTO, error)
	ListLanguageTestRequirements(ctx context.Context, dest pgtype.UUID, cat string) ([]languagetesthub.RequirementDTO, error)
	FindScholarships(ctx context.Context, f scholarships.ScholarshipFilters) ([]scholarships.ScholarshipResult, int64, error)
	ListUniversities(ctx context.Context, search, country, sort string, limit, offset int32) ([]gen.ListSpacesByTypePaginatedRow, int64, error)
	GetUniversity(ctx context.Context, slug string) (gen.GetSpaceBySlugRow, error)
	GetDestinationBrief(ctx context.Context, slug string, include []string) (*publicapi.DestinationBrief, error)
}
```

Then handlers following the `ScholarshipSearchHandler` shape — args struct,
`json.Unmarshal`, cap limit at 50, call the reader, `successToolResult` /
`errorToolResult`:

```go
type visaSearchArgs struct {
	DestSpaceID string `json:"destination_space_id,omitempty"`
	Profession  string `json:"profession,omitempty"`
	Education   string `json:"education,omitempty"`
	Years       int32  `json:"experience_years,omitempty"`
	Limit       int32  `json:"limit,omitempty"`
	Page        int32  `json:"page,omitempty"`
}

func VisaSearchHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.PublicAPI == nil {
			return errorToolResult("visa_search_unavailable", "visa search is not configured"), nil
		}
		var a visaSearchArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Limit <= 0 || a.Limit > 50 {
			a.Limit = 20
		}
		if a.Page <= 0 {
			a.Page = 1
		}
		var dest pgtype.UUID
		if a.DestSpaceID != "" {
			if err := dest.Scan(a.DestSpaceID); err != nil {
				return errorToolResult("invalid_arguments", "invalid destination_space_id"), nil
			}
		}
		routes, total, err := deps.PublicAPI.ListVisaRoutes(ctx, dest, a.Profession, a.Education, a.Years, a.Page, a.Limit)
		if err != nil {
			return errorToolResult("visa_search_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"routes": routes, "total": total}), nil
	}
}
```

Implement `VisaGetHandler` (`{"route_id"}` → `GetVisaRoute`),
`VisaCompareHandler` (`{"route_ids": [...]}` max 5 → `CompareVisaRoutes`),
`UniversityGetHandler` (`{"slug"}` → `GetUniversity`), and
`DestinationBriefHandler` (`{"slug", "include"}` → `GetDestinationBrief`)
the same way.

Register in `tools_register.go` — append inside `RegisterPhase9Tools`:

```go
	// --- Agent data tools (spec 2026-09-23-agent-data-api) ---
	registry.RegisterHandler("visa.search", VisaSearchHandler(deps))
	registry.RegisterHandler("visa.get", VisaGetHandler(deps))
	registry.RegisterHandler("visa.compare", VisaCompareHandler(deps))
	registry.RegisterHandler("university.get", UniversityGetHandler(deps))
	registry.RegisterHandler("destination.brief", DestinationBriefHandler(deps))
```

Wire in `server.go` after `agentDeps` is built (~line 1960):

```go
	agentDeps.PublicAPI = publicAPIService
```

- [ ] **Step 4: Seed migration `000722_seed_ai_tools_agent_data.up.sql`**

Mirror `migrations/000388_seed_ai_agent_tools_v1.up.sql` exactly — `BEGIN;`
… `INSERT INTO ai_tools (tenant_id, name, description, category, config,
is_enabled, required_permission) VALUES (...)` … `COMMIT;` with
`tenant_id '00000000-0000-0000-0000-000000000001'`, `category 'agent_data'`,
`required_permission NULL` (read-only; `agent.use` gates at the door),
`is_enabled true`. One row per tool with a `config` JSONB holding
`function_name`, `description`, `parameters` (JSON Schema matching the args
structs above), `handler` reference string, `reads_user_data false`,
`writes_user_data false`, `requires_hitl false`, `is_blocked false`.
Down migration: `DELETE FROM ai_tools WHERE name IN ('visa.search','visa.get','visa.compare','university.get','destination.brief');`

Check the migration runner naming/numbering convention (latest is 000721;
use 000722 unless taken — adjust if a newer file landed since).

- [ ] **Step 5: Run tests + build + migration check**

Run: `go test ./internal/ai/ ./internal/mcp/ -v && go build ./internal/server/ && make migrate-up` (or the repo's local migration target — check `Makefile`)
Expected: PASS; migration applies cleanly

- [ ] **Step 6: Commit**

```bash
git add internal/ai/tools_moat.go internal/ai/tools_moat_test.go internal/ai/tools_deps.go internal/ai/tools_register.go internal/server/server.go migrations/000722_seed_ai_tools_agent_data.*
git commit -m "feat(ai): agent data tools for visa, university, destination brief"
```

---

### Task 9: OpenAPI spec + catalogs + final verification

**Files:**
- Modify: `internal/publicapi/openapi.go` (new paths + tags + `x-required-tier` extension on premium ops)
- Modify: `internal/publicapi/openapi_test.go` (extend `wantPaths`)
- Modify: `internal/publicapi/public_catalog_openapi.json` — **regenerate** via `python3 scripts/generate_openapi_public_catalog_json.py` (check the script's input source first; if it derives from `api-routes-catalog.md`, Task 1 already updated it)

- [ ] **Step 1: Update the OpenAPI test to fail**

Add to `wantPaths` in `openapi_test.go`:

```go
		"/public/visa-routes",
		"/public/visa-routes/{id}",
		"/public/visa-routes/compare",
		"/public/visa-timelines/stats",
		"/public/visa-timelines/stats/trend",
		"/public/credentials",
		"/public/credentials/{id}",
		"/public/credentials/professions",
		"/public/language-tests",
		"/public/language-tests/{slug}",
		"/public/language-tests/requirements",
		"/public/scholarships",
		"/public/scholarships/{id}",
		"/public/scholarships/statuses",
		"/public/scholarships/levels",
		"/public/universities",
		"/public/universities/{slug}",
		"/public/destinations/{slug}/brief",
```

Run: `go test ./internal/publicapi/ -run TestGetPublicOpenAPISpec -v`
Expected: FAIL — paths missing

- [ ] **Step 2: Extend `openapi.go`**

- Add tags: `"Visa & Immigration"`, `"Education"`, `"Destinations"` in `buildTags()`.
- Add one path-builder function per route following the existing
  `trendingEndpoint`/`spacesPath` pattern, each calling `operation(...)` with
  a stable `operationId` (`listVisaRoutes`, `getVisaRoute`, `compareVisaRoutes`,
  `getVisaTimelineStats`, `getVisaTimelineTrend`, `listCredentialPathways`,
  `getCredentialPathway`, `listCredentialProfessions`, `listLanguageTests`,
  `getLanguageTest`, `listLanguageTestRequirements`, `findScholarships`,
  `getScholarship`, `listScholarshipStatuses`, `listScholarshipLevels`,
  `listUniversities`, `getUniversity`, `getDestinationBrief`), correct
  `x-required-scope`, and a query-parameter list mirroring the handlers.
- For premium ops, add `"x-required-tier": "pro"` into the op map after
  `operation(...)` returns.
- Register all in `buildPublicPaths()`.

- [ ] **Step 3: Regenerate the catalog artifact**

Run: `python3 scripts/generate_openapi_public_catalog_json.py`
(Inspect the script first — if it reads `api-routes-catalog.md`, Task 1's
entries flow through automatically; if it has its own route list, add the new
routes there.)

- [ ] **Step 4: Full verification**

Run:
```bash
go build ./...
go test ./internal/publicapi/ ./internal/mcp/ ./internal/ai/ -v
go vet ./internal/publicapi/ ./internal/mcp/ ./internal/ai/ ./internal/server/
```
Expected: build clean, all tests pass, vet clean

Smoke (if a local DB is up — otherwise note as manual verification):
```bash
curl -H "X-API-Key: jc_test_..." localhost:8080/api/v1/public/visa-routes | jq .
curl -H "X-API-Key: jc_test_..." localhost:8080/api/v1/public/destinations/canada/brief | jq .
```

- [ ] **Step 5: Commit**

```bash
git add internal/publicapi/openapi.go internal/publicapi/openapi_test.go internal/publicapi/public_catalog_openapi.json
git commit -m "feat(publicapi): OpenAPI catalog for agent data endpoints"
```

---

## Post-implementation checklist

- [ ] All 18 `/public/*` routes respond under scoped API-key auth
- [ ] Premium routes return `403 tier_required` for `regular`-tier key owners
- [ ] `agent.use` appears in `/developer/scopes` and `/control/api-scopes`
- [ ] MCP `tools/list` with X-API-Key returns only allowlisted tools
- [ ] `/developer/swagger` renders all new operations with scopes/tiers
- [ ] Spec file updated if implementation diverged anywhere (Rule: JEOS stays in sync)
