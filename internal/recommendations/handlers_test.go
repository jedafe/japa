package recommendations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/visitor"
)

// mockSettings is a minimal settingsReader for tests. It returns whatever
// is in the `strings`/`ints` maps, falling back to the provided default.
type mockSettings struct {
	strings map[string]string
	ints    map[string]int
}

func (m mockSettings) GetStringOr(_ context.Context, _ pgtype.UUID, key, fb string) string {
	if v, ok := m.strings[key]; ok {
		return v
	}

	return fb
}

func (m mockSettings) GetIntOr(_ context.Context, _ pgtype.UUID, key string, fb int) int {
	if v, ok := m.ints[key]; ok {
		return v
	}

	return fb
}

// mockTrending is a minimal TrendingReader for tests.
type mockTrending struct {
	content []gen.GetTrendingContentRow
	spaces  []gen.GetTrendingSpacesRow
	levels  map[[16]byte]string
	err     error
}

func (m mockTrending) GetTrendingContent(context.Context, pgtype.UUID, string, string, int32) ([]gen.GetTrendingContentRow, error) {
	return m.content, m.err
}

func (m mockTrending) GetTrendingSpaces(context.Context, pgtype.UUID, string, string, int32) ([]gen.GetTrendingSpacesRow, error) {
	return m.spaces, m.err
}

func (m mockTrending) GetSpaceLevels(_ context.Context, _ pgtype.UUID, _ []pgtype.UUID) (map[[16]byte]string, error) {
	return m.levels, nil
}

func validUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Valid: true}
}

// reqWithTenantAndVisitor builds a GET /recommendations request with the
// tenant and a VisitorContext injected into the context (mirrors what the
// tenant + visitor middleware set in production).
func reqWithTenantAndVisitor(vc visitor.VisitorContext, query string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations?"+query, nil)
	ctx := context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: validUUID()})
	ctx = context.WithValue(ctx, visitor.ContextKey{}, vc)

	return req.WithContext(ctx)
}

// --- classifySlugs --------------------------------------------------------

func TestClassifySlugs_SplitsToolsAndFeatures(t *testing.T) {
	t.Parallel()

	slugs := []string{
		"visa-route-comparison", // tool
		"story-wall",            // feature
		"embassy-alerter",       // tool
		"destination-wiki",      // feature
		"mentorship",            // feature
		"tax-calculator",        // tool
	}

	tools, features := classifySlugs(slugs)
	if len(tools) != 3 || tools[0] != "visa-route-comparison" || tools[1] != "embassy-alerter" || tools[2] != "tax-calculator" {
		t.Errorf("tools = %v, want [visa-route-comparison embassy-alerter tax-calculator]", tools)
	}

	if len(features) != 3 || features[0] != "story-wall" || features[1] != "destination-wiki" || features[2] != "mentorship" {
		t.Errorf("features = %v, want [story-wall destination-wiki mentorship]", features)
	}
}

func TestClassifySlugs_Empty(t *testing.T) {
	t.Parallel()

	tools, features := classifySlugs(nil)
	if len(tools) != 0 || len(features) != 0 {
		t.Errorf("expected empty slices, got tools=%v features=%v", tools, features)
	}
}

// --- defaultToolSlugs -----------------------------------------------------

func TestDefaultToolSlugs_OriginVisitorPlanning(t *testing.T) {
	t.Parallel()

	got := defaultToolSlugs(visitor.VisitorContext{Archetype: visitor.ArchetypeOriginVisitor, JapaStage: "planning"})
	want := []string{"visa-route-comparison", "embassy-alerter", "processing-time-tracker", "credential-checker"}

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestDefaultToolSlugs_DiasporaSettled(t *testing.T) {
	t.Parallel()

	got := defaultToolSlugs(visitor.VisitorContext{Archetype: visitor.ArchetypeDiaspora, JapaStage: "settled"})
	want := []string{"tax-calculator", "attestation-guide", "community-locator", "mutual-aid-board"}

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestDefaultToolSlugs_Default(t *testing.T) {
	t.Parallel()

	got := defaultToolSlugs(visitor.VisitorContext{Archetype: visitor.ArchetypeGlobalTraveler})
	if len(got) == 0 {
		t.Fatal("expected non-empty default slugs for global_traveler")
	}
}

// --- resolveToolSlugs (matrix resolution) ---------------------------------

func TestResolveToolSlugs_ExactMatchWins(t *testing.T) {
	t.Parallel()

	matrix := `{
		"origin_visitor:planning": ["visa-route-comparison"],
		"origin_visitor:*": ["embassy-alerter"],
		"*:planning": ["processing-time-tracker"],
		"*:*": ["destination-wiki"]
	}`
	h := &Handlers{settings: mockSettings{strings: map[string]string{"geo_personalization.stage_tool_matrix": matrix}}}
	got := h.resolveToolSlugs(t.Context(), validUUID(), visitor.VisitorContext{Archetype: visitor.ArchetypeOriginVisitor, JapaStage: "planning"})
	// Exact match returns all 4 in priority order (exact first, then wildcard merge).
	if len(got) != 4 || got[0] != "visa-route-comparison" {
		t.Errorf("got %v, want 4 slugs with visa-route-comparison first", got)
	}
}

func TestResolveToolSlugs_ArchetypeWildcardFallback(t *testing.T) {
	t.Parallel()

	matrix := `{
		"origin_visitor:*": ["embassy-alerter", "visa-route-comparison"],
		"*:*": ["destination-wiki"]
	}`
	h := &Handlers{settings: mockSettings{strings: map[string]string{"geo_personalization.stage_tool_matrix": matrix}}}

	got := h.resolveToolSlugs(t.Context(), validUUID(), visitor.VisitorContext{Archetype: visitor.ArchetypeOriginVisitor, JapaStage: "unknown_stage"})
	if len(got) != 3 || got[0] != "embassy-alerter" {
		t.Errorf("got %v, want archetype-wildcard fallback", got)
	}
}

func TestResolveToolSlugs_StarStarFallback(t *testing.T) {
	t.Parallel()

	matrix := `{"*:*": ["destination-wiki", "community-locator"]}`
	h := &Handlers{settings: mockSettings{strings: map[string]string{"geo_personalization.stage_tool_matrix": matrix}}}

	got := h.resolveToolSlugs(t.Context(), validUUID(), visitor.VisitorContext{Archetype: visitor.ArchetypeGlobalTraveler, JapaStage: "unknown"})
	if len(got) != 2 || got[0] != "destination-wiki" {
		t.Errorf("got %v, want *:* fallback", got)
	}
}

func TestResolveToolSlugs_InvalidJSONFallsBack(t *testing.T) {
	t.Parallel()

	h := &Handlers{settings: mockSettings{strings: map[string]string{"geo_personalization.stage_tool_matrix": "not-json"}}}
	got := h.resolveToolSlugs(t.Context(), validUUID(), visitor.VisitorContext{Archetype: visitor.ArchetypeOriginVisitor, JapaStage: "planning"})
	// Should fall back to defaultToolSlugs.
	if len(got) == 0 {
		t.Error("expected fallback slugs on invalid JSON")
	}
}

func TestResolveToolSlugs_EmptySettingFallsBack(t *testing.T) {
	t.Parallel()

	h := &Handlers{settings: mockSettings{}}

	got := h.resolveToolSlugs(t.Context(), validUUID(), visitor.VisitorContext{Archetype: visitor.ArchetypeDiaspora, JapaStage: "settled"})
	if len(got) == 0 {
		t.Error("expected fallback slugs on empty setting")
	}
}

func TestResolveToolSlugs_NilSettingsFallsBack(t *testing.T) {
	t.Parallel()

	h := &Handlers{}

	got := h.resolveToolSlugs(t.Context(), validUUID(), visitor.VisitorContext{Archetype: visitor.ArchetypeOriginVisitor, JapaStage: "planning"})
	if len(got) == 0 {
		t.Error("expected fallback slugs on nil settings")
	}
}

// --- contentURL -----------------------------------------------------------

func TestContentURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		contentType, slug, want string
	}{
		{"blog_post", "my-post", "/blog/my-post"},
		{"forum_thread", "my-thread", "/forum/my-thread"},
		{"story", "my-story", "/content/my-story"},
		{"", "no-type", "/content/no-type"},
	}
	for _, c := range cases {
		if got := contentURL(c.contentType, c.slug); got != c.want {
			t.Errorf("contentURL(%q, %q) = %q, want %q", c.contentType, c.slug, got, c.want)
		}
	}
}

// --- sliceItems -----------------------------------------------------------

func TestSliceItems(t *testing.T) {
	t.Parallel()

	items := []RecommendationItem{{Slug: "a"}, {Slug: "b"}, {Slug: "c"}, {Slug: "d"}}
	if got := sliceItems(items, 2); len(got) != 2 || got[1].Slug != "b" {
		t.Errorf("sliceItems(4, 2) = %v, want first 2", got)
	}

	if got := sliceItems(items, 10); len(got) != 4 {
		t.Errorf("sliceItems(4, 10) = %v, want all 4", got)
	}

	if got := sliceItems(items, 0); len(got) != 0 {
		t.Errorf("sliceItems(4, 0) = %v, want empty", got)
	}
}

// --- resolveLimit / resolveCacheTTL ---------------------------------------

func TestResolveLimit_DefaultAndClamp(t *testing.T) {
	t.Parallel()
	// max=5: the default (10) is clamped down to 5; explicit 3 is under the cap.
	h := &Handlers{settings: mockSettings{ints: map[string]int{"geo_personalization.recommendations_max_limit": 5}}}
	if got := h.resolveLimit(t.Context(), validUUID(), ""); got != 5 {
		t.Errorf("default limit clamped to max = %d, want 5", got)
	}

	if got := h.resolveLimit(t.Context(), validUUID(), "100"); got != 5 {
		t.Errorf("clamped limit = %d, want 5", got)
	}

	if got := h.resolveLimit(t.Context(), validUUID(), "3"); got != 3 {
		t.Errorf("limit = %d, want 3", got)
	}
}

func TestResolveLimit_DefaultMaxAllowsDefaultLimit(t *testing.T) {
	t.Parallel()
	// max=20 (default): the default limit 10 is under the cap.
	h := &Handlers{settings: mockSettings{ints: map[string]int{"geo_personalization.recommendations_max_limit": 20}}}
	if got := h.resolveLimit(t.Context(), validUUID(), ""); got != 10 {
		t.Errorf("default limit = %d, want 10", got)
	}
}

func TestResolveLimit_NilSettingsDefaultMax(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	if got := h.resolveLimit(t.Context(), validUUID(), "100"); got != defaultMaxLimit {
		t.Errorf("clamped limit = %d, want default %d", got, defaultMaxLimit)
	}
}

func TestResolveCacheTTL_Configurable(t *testing.T) {
	t.Parallel()

	h := &Handlers{settings: mockSettings{ints: map[string]int{"geo_personalization.recommendations_cache_ttl_seconds": 300}}}
	if got := h.resolveCacheTTL(t.Context(), validUUID()); got.Seconds() != 300 {
		t.Errorf("cache TTL = %v, want 300s", got)
	}
}

func TestResolveCacheTTL_NilSettingsDefault(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	if got := h.resolveCacheTTL(t.Context(), validUUID()); got != defaultCacheTTL {
		t.Errorf("cache TTL = %v, want default %v", got, defaultCacheTTL)
	}
}

// --- GetRecommendations end-to-end (response shape) ----------------------

// runRecommendations invokes GetRecommendations with the given handler, VC,
// and query string, returning the decoded response envelope.
func runRecommendations(t *testing.T, h *Handlers, vc visitor.VisitorContext, query string) RecommendationResponse {
	t.Helper()

	req := reqWithTenantAndVisitor(vc, query)
	rec := httptest.NewRecorder()
	h.GetRecommendations(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var envelope struct {
		Data RecommendationResponse `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal error: %v; body=%s", err, rec.Body.String())
	}

	return envelope.Data
}

func TestGetRecommendations_ToolsAndFeaturesSplit(t *testing.T) {
	t.Parallel()

	h := &Handlers{
		settings: mockSettings{strings: map[string]string{
			"geo_personalization.stage_tool_matrix": `{"origin_visitor:planning": ["visa-route-comparison", "story-wall"]}`,
		}},
		trending: mockTrending{},
	}
	resp := runRecommendations(t, h, visitor.VisitorContext{Archetype: visitor.ArchetypeOriginVisitor, JapaStage: "planning"}, "limit=5")

	if len(resp.Tools) == 0 || resp.Tools[0].Slug != "visa-route-comparison" {
		t.Errorf("tools = %v, want visa-route-comparison first", resp.Tools)
	}

	if len(resp.Features) == 0 || resp.Features[0].Slug != "story-wall" {
		t.Errorf("features = %v, expected story-wall first", resp.Features)
	}
}

func TestGetRecommendations_ContentAndSpacesFromTrending(t *testing.T) {
	t.Parallel()

	h := &Handlers{
		settings: mockSettings{strings: map[string]string{
			"geo_personalization.stage_tool_matrix": `{"*:*": ["destination-wiki"]}`,
		}},
		trending: mockTrending{
			content: []gen.GetTrendingContentRow{{ID_2: validUUID(), Title: "Test Post", Slug: "test-post", ContentType: "blog_post"}},
			spaces:  []gen.GetTrendingSpacesRow{{ID_2: validUUID(), Name: "Nigeria", Slug: "nigeria"}},
		},
	}
	resp := runRecommendations(t, h, visitor.VisitorContext{Archetype: visitor.ArchetypeGlobalTraveler}, "limit=5")

	if len(resp.Content) != 1 || resp.Content[0].Title != "Test Post" {
		t.Errorf("content = %v, expected 1 item titled 'Test Post'", resp.Content)
	}

	if resp.Content[0].URL != "/blog/test-post" {
		t.Errorf("content[0].url = %q, want /blog/test-post", resp.Content[0].URL)
	}

	if len(resp.Spaces) != 1 || resp.Spaces[0].Name != "Nigeria" {
		t.Errorf("spaces = %v, expected 1 item named 'Nigeria'", resp.Spaces)
	}
}

func TestGetRecommendations_ArraysNeverNull(t *testing.T) {
	t.Parallel()

	h := &Handlers{
		settings: mockSettings{strings: map[string]string{
			"geo_personalization.stage_tool_matrix": `{"*:*": ["destination-wiki"]}`,
		}},
		trending: mockTrending{},
	}
	req := reqWithTenantAndVisitor(visitor.VisitorContext{Archetype: visitor.ArchetypeGlobalTraveler}, "limit=5")
	rec := httptest.NewRecorder()
	h.GetRecommendations(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	raw := rec.Body.String()
	for _, marker := range []string{`"tools":null`, `"features":null`, `"content":null`, `"spaces":null`} {
		if contains(raw, marker) {
			t.Errorf("body contains %q — array serialized as null, want []", marker)
		}
	}
}

func TestGetRecommendations_LimitTruncatesTools(t *testing.T) {
	t.Parallel()

	matrix := `{"*:*": ["visa-route-comparison", "embassy-alerter", "processing-time-tracker", "credential-checker", "housing-guide"]}`
	h := &Handlers{
		settings: mockSettings{strings: map[string]string{"geo_personalization.stage_tool_matrix": matrix}},
		trending: mockTrending{},
	}

	resp := runRecommendations(t, h, visitor.VisitorContext{Archetype: visitor.ArchetypeGlobalTraveler}, "limit=2")
	if len(resp.Tools) > 2 {
		t.Errorf("tools len = %d, want <= 2 (limit)", len(resp.Tools))
	}
}

func TestGetRecommendations_TenantUnavailable(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations", nil)
	rec := httptest.NewRecorder()
	h.GetRecommendations(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

// --- buildContentItems / buildSpaceItems (nil trending) ------------------

func TestBuildContentItems_NilTrendingReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	h := &Handlers{}

	got := h.buildContentItems(t.Context(), validUUID(), visitor.VisitorContext{}, 5)
	if got == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}

	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d items", len(got))
	}
}

func TestBuildSpaceItems_NilTrendingReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	h := &Handlers{}

	got := h.buildSpaceItems(t.Context(), validUUID(), visitor.VisitorContext{}, 5)
	if got == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}

	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d items", len(got))
	}
}

// contains reports whether s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}

	return -1
}
