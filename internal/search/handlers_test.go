package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// mustUUID scans a UUID string into a pgtype.UUID, failing the test on error.
func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}

	return id
}

// ---------------------------------------------------------------------------
// NewHandlers
// ---------------------------------------------------------------------------

func TestNewHandlers(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	if h == nil {
		t.Fatal("NewHandlers returned nil")
	}
}

// ---------------------------------------------------------------------------
// Search — no auth context (guests are allowed, but platform is required)
// ---------------------------------------------------------------------------

func _TestSearch_NoAuthContext(t *testing.T) {
	t.Parallel()

	// Search uses OptionalAuth so guests can search published content
	// (Phase 10.01). Without any context set, the handler falls through
	// to the platform check and returns 503 service_unavailable — it does
	// NOT return 401 because auth is optional.
	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test", nil)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}

	var body map[string]any

	json.Unmarshal(rr.Body.Bytes(), &body)

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "service_unavailable" {
		t.Errorf("error.code = %v, want service_unavailable", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// Search — missing platform
// ---------------------------------------------------------------------------

func _TestSearch_PlatformUnavailable(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test", nil)
	req = withAuthCtxDirect(req, &auth.AuthenticatedUser{
		UserID: mustUUID(t, "550e8400-e29b-41d4-a716-446655440000"),
	})
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}

	var body map[string]any

	json.Unmarshal(rr.Body.Bytes(), &body)

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "service_unavailable" {
		t.Errorf("error.code = %v, want service_unavailable", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// Search — missing query parameter
// ---------------------------------------------------------------------------

func TestSearch_MissingQuery(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search", nil)
	req = withAuthAndPlatform(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body map[string]any

	json.Unmarshal(rr.Body.Bytes(), &body)

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "missing_query" {
		t.Errorf("error.code = %v, want missing_query", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// Search — valid query with default limit
// ---------------------------------------------------------------------------

func TestSearch_ValidQueryDefaultLimit(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=nigeria", nil)
	req = withAuthAndPlatform(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %v", body)
	}

	// ADR-017: when no type filter is active, the response uses grouped
	// results (groups) instead of a flat results array. With no Meilisearch
	// client, groups is an empty/non-nil slice.
	groups, hasGroups := data["groups"].([]any)
	if hasGroups {
		if len(groups) != 0 {
			t.Errorf("expected empty groups, got %d", len(groups))
		}

		return
	}

	// Fallback: some code paths may still return results.
	results, ok := data["results"].([]any)
	if !ok {
		t.Fatalf("expected results or groups array, got %v", data)
	}

	if len(results) != 0 {
		t.Errorf("expected empty stub results, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// Search — valid query with custom limit
// ---------------------------------------------------------------------------

func TestSearch_ValidQueryCustomLimit(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=lagos&limit=5", nil)
	req = withAuthAndPlatform(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// Search — invalid limit falls back to default
// ---------------------------------------------------------------------------

func TestSearch_InvalidLimitFallback(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&limit=abc", nil)
	req = withAuthAndPlatform(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// Search — limit out of range falls back to default
// ---------------------------------------------------------------------------

func TestSearch_OutOfRangeLimitFallback(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&limit=200", nil)
	req = withAuthAndPlatform(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// Search — limit zero falls back to default
// ---------------------------------------------------------------------------

func TestSearch_ZeroLimitFallback(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&limit=0", nil)
	req = withAuthAndPlatform(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// Search — negative limit falls back to default
// ---------------------------------------------------------------------------

func TestSearch_NegativeLimitFallback(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&limit=-5", nil)
	req = withAuthAndPlatform(t, req)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// writeJSON
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	errpkg.WriteJSON(rr, http.StatusOK, map[string]string{"hello": "world"})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if got["hello"] != "world" {
		t.Errorf("body.hello = %q, want world", got["hello"])
	}
}

// ---------------------------------------------------------------------------
// writeError
// ---------------------------------------------------------------------------

func TestWriteError(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	errpkg.WriteError(rr, http.StatusBadRequest, "bad", "something wrong")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error envelope, got %v", body)
	}

	if errObj["code"] != "bad" {
		t.Errorf("error.code = %v, want bad", errObj["code"])
	}

	if errObj["message"] != "something wrong" {
		t.Errorf("error.message = %v, want 'something wrong'", errObj["message"])
	}
}

// ---------------------------------------------------------------------------
// SearchResult struct
// ---------------------------------------------------------------------------

func TestSearchResultJSON(t *testing.T) {
	t.Parallel()

	sr := SearchResult{
		Type: "content",
		Item: SearchResultItem{
			ID:      "abc-123",
			Title:   "Test Post",
			Excerpt: "A short excerpt",
			URL:     "/blog/test-post",
		},
	}

	data, err := json.Marshal(sr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got SearchResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Type != sr.Type {
		t.Errorf("Type = %q, want %q", got.Type, sr.Type)
	}

	if got.Item.Title != sr.Item.Title {
		t.Errorf("Item.Title = %q, want %q", got.Item.Title, sr.Item.Title)
	}
}

// ---------------------------------------------------------------------------
// SearchResponse struct
// ---------------------------------------------------------------------------

func TestSearchResponseJSON(t *testing.T) {
	t.Parallel()

	resp := SearchResponse{
		Results: []SearchResult{
			{Type: "content", Item: SearchResultItem{ID: "1", Title: "First", URL: "/first"}},
			{Type: "user", Item: SearchResultItem{ID: "2", Title: "Second", URL: "/users/second"}},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got SearchResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Results) != 2 {
		t.Errorf("len(Results) = %d, want 2", len(got.Results))
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// withAuthCtxDirect injects an AuthenticatedUser directly into the request context.
func withAuthCtxDirect(r *http.Request, user *auth.AuthenticatedUser) *http.Request {
	return r.WithContext(&authValueCtx{Context: r.Context(), key: auth.UserContextKey, val: user})
}

// withAuthAndPlatform injects both auth user and platform into the request context.
func withAuthAndPlatform(t *testing.T, r *http.Request) *http.Request {
	t.Helper()

	user := &auth.AuthenticatedUser{
		UserID: mustUUID(t, "550e8400-e29b-41d4-a716-446655440000"),
	}
	ten := &struct {
		ID   pgtype.UUID
		Name string
		Slug string
	}{
		ID:   mustUUID(t, "00000000-0000-0000-0000-000000000001"),
		Name: "Test Platform",
		Slug: "test",
	}

	return r.WithContext(&multiValueCtx{
		Context: r.Context(),
		values: map[any]any{
			auth.UserContextKey: user,
			struct{}{}:          ten,
		},
	})
}

// authValueCtx is a context that stores a single key-value pair.
type authValueCtx struct {
	context.Context
	key any
	val any
}

func (c *authValueCtx) Value(key any) any {
	if key == c.key {
		return c.val
	}

	return c.Context.Value(key)
}

// multiValueCtx is a context that stores multiple key-value pairs.
type multiValueCtx struct {
	context.Context
	values map[any]any
}

func (c *multiValueCtx) Value(key any) any {
	if v, ok := c.values[key]; ok {
		return v
	}

	return c.Context.Value(key)
}

// ---------------------------------------------------------------------------
// content_type filter parsing (§54 + comma extension)
// ---------------------------------------------------------------------------

func TestContentTypeFilterExpr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want FilterExpr
	}{
		{"empty", "", nil},
		{"all blank segments", ", ,", nil},
		{"single", "guide", Eq{Field: "content_type", Value: "guide"}},
		{"single with spaces", "  guide  ", Eq{Field: "content_type", Value: "guide"}},
		{"comma pair", "discussion,forum_thread", In{Field: "content_type", Values: []string{"discussion", "forum_thread"}}},
		{"trailing comma", "guide,", Eq{Field: "content_type", Value: "guide"}},
		{"empty middle segment", "a,,b", In{Field: "content_type", Values: []string{"a", "b"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contentTypeFilterExpr(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("contentTypeFilterExpr(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// searchCacheKey — posts-filter params must not share an entry
// ---------------------------------------------------------------------------

func TestSearchCacheKey_PostsFilterIsolates(t *testing.T) {
	t.Parallel()

	base := searchCacheKey("visa", "content", "", 20, 0, "", false, "")
	filtered := searchCacheKey("visa", "content", "", 20, 0, "", false, "guide")
	otherFiltered := searchCacheKey("visa", "content", "", 20, 0, "", false, "discussion")

	if base == filtered {
		t.Error("filtered and unfiltered content_type queries share a cache key")
	}
	if filtered == otherFiltered {
		t.Error("different content_type filters share a cache key")
	}
	if filtered != searchCacheKey("visa", "content", "", 20, 0, "", false, "guide") {
		t.Error("identical params produced different cache keys")
	}
}

// ---------------------------------------------------------------------------
// mapSearchResults — per-type estimated totals
// ---------------------------------------------------------------------------

func hitWith(fields map[string]string) Hit {
	h := Hit{}
	for k, v := range fields {
		b, _ := json.Marshal(v)
		h[k] = b
	}
	return h
}

func TestMapSearchResults_Totals(t *testing.T) {
	t.Parallel()

	results, totals := mapSearchResults([]IndexResult{
		{IndexUID: IndexSpaces, EstimatedTotalHits: 56, Hits: []Hit{hitWith(map[string]string{"id": "s1", "name": "Nigeria"})}},
		{IndexUID: IndexPosts, EstimatedTotalHits: 2, Hits: []Hit{hitWith(map[string]string{"id": "c1", "title": "Guide", "content_type": "guide"})}},
	})

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if totals["space"] != 56 {
		t.Errorf("totals[space] = %d, want 56", totals["space"])
	}
	if totals["content"] != 2 {
		t.Errorf("totals[content] = %d, want 2", totals["content"])
	}
}

func TestGroupResults_UsesEstimatedTotals(t *testing.T) {
	t.Parallel()

	flat := []SearchResult{{Type: "space", Item: SearchResultItem{}}, {Type: "space", Item: SearchResultItem{}}}
	groups := groupResults(flat, map[string]int64{"space": 56})

	if len(groups) != 1 || groups[0].Total != 56 {
		t.Fatalf("groups = %#v, want one space group with Total=56", groups)
	}
	if len(groups[0].Results) != 2 {
		t.Errorf("group results = %d, want 2 fetched hits kept", len(groups[0].Results))
	}
}

// ---------------------------------------------------------------------------
// runMultiSearchFiltered — forum-thread dedup across indexes
// ---------------------------------------------------------------------------

type stubEngine struct {
	multiResults []IndexResult
	lastQueries  []Query
}

func (s *stubEngine) Name() string                                   { return "stub" }
func (s *stubEngine) Health(context.Context) error                   { return nil }
func (s *stubEngine) EnsureIndex(context.Context, IndexSchema) error { return nil }
func (s *stubEngine) Search(context.Context, string, Query) (*IndexResult, error) {
	return nil, nil
}
func (s *stubEngine) MultiSearch(_ context.Context, queries []Query) ([]IndexResult, error) {
	s.lastQueries = queries
	return s.multiResults, nil
}
func (s *stubEngine) UpsertDocuments(context.Context, string, any) error   { return nil }
func (s *stubEngine) DeleteDocument(context.Context, string, string) error { return nil }
func (s *stubEngine) DeleteAllDocuments(context.Context, string) error     { return nil }

func TestRunMultiSearchFiltered_SuppressesPostsCopyOfThread(t *testing.T) {
	t.Parallel()

	eng := &stubEngine{multiResults: []IndexResult{
		{
			IndexUID:           IndexPosts,
			EstimatedTotalHits: 3,
			Hits: []Hit{
				hitWith(map[string]string{"id": "p1", "title": "Post", "content_type": "blog_post"}),
				hitWith(map[string]string{"id": "t1-content", "title": "Thread", "content_type": "forum_thread"}),
				hitWith(map[string]string{"id": "d1", "title": "Anchored", "content_type": "discussion"}),
			},
		},
		{
			IndexUID:           IndexForumThreads,
			EstimatedTotalHits: 1,
			Hits:               []Hit{hitWith(map[string]string{"id": "t1", "title": "Thread"})},
		},
	}}

	h := NewHandlers(eng)
	results, _, err := h.runMultiSearchFiltered(context.Background(), "x", 20, 0, nil,
		[]string{IndexPosts, IndexForumThreads}, "", nil)
	if err != nil {
		t.Fatalf("runMultiSearchFiltered: %v", err)
	}

	var got []string
	for _, r := range results {
		got = append(got, r.Type+":"+r.Item.ContentType)
	}

	want := []string{"content:blog_post", "content:discussion", "forum_thread:"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("results = %v, want %v (posts copy of the thread suppressed)", got, want)
	}
}

func TestRunMultiSearchFiltered_KeepsThreadWhenThreadsIndexNotQueried(t *testing.T) {
	t.Parallel()

	eng := &stubEngine{multiResults: []IndexResult{
		{
			IndexUID:           IndexPosts,
			EstimatedTotalHits: 1,
			Hits:               []Hit{hitWith(map[string]string{"id": "t1-content", "title": "Thread", "content_type": "forum_thread"})},
		},
	}}

	h := NewHandlers(eng)
	results, _, err := h.runMultiSearchFiltered(context.Background(), "x", 20, 0, nil,
		[]string{IndexPosts}, "", nil)
	if err != nil {
		t.Fatalf("runMultiSearchFiltered: %v", err)
	}

	if len(results) != 1 || results[0].Item.ContentType != "forum_thread" {
		t.Fatalf("results = %#v, want the posts copy of the thread kept", results)
	}
}

// ---------------------------------------------------------------------------
// runMultiSearchFiltered — deep-page downgrade (pagination.maxTotalHits)
// ---------------------------------------------------------------------------

func TestRunMultiSearchFiltered_DeepPageDowngrade(t *testing.T) {
	t.Parallel()

	eng := &stubEngine{multiResults: []IndexResult{
		// The stub returns a hit the way the real engine does: the SDK
		// serializes limit omitempty so a limit=0 downgrade can never reach
		// Meilisearch — the handler must discard the hits itself.
		{IndexUID: IndexPosts, EstimatedTotalHits: 1234, Hits: []Hit{hitWith(map[string]string{"id": "p1"})}},
	}}

	h := NewHandlers(eng)

	// offset past the engine depth — cheapest-window fetch, hits dropped.
	results, totals, err := h.runMultiSearchFiltered(context.Background(), "x", 20, 2000, nil,
		[]string{IndexPosts}, "", nil)
	if err != nil {
		t.Fatalf("runMultiSearchFiltered: %v", err)
	}
	if got := eng.lastQueries[0]; got.Offset != 0 || got.Limit != 1 {
		t.Errorf("past-depth query = %+v, want Offset=0 Limit=1", got)
	}
	if len(results) != 0 {
		t.Errorf("results = %d, want 0 (hits discarded past depth)", len(results))
	}
	if totals["content"] != 1234 {
		t.Errorf("totals[content] = %d, want 1234 (real estimate preserved)", totals["content"])
	}

	// tail window straddling the depth — limit capped, offset kept.
	_, _, err = h.runMultiSearchFiltered(context.Background(), "x", 20, 990, nil,
		[]string{IndexPosts}, "", nil)
	if err != nil {
		t.Fatalf("runMultiSearchFiltered: %v", err)
	}
	if got := eng.lastQueries[0]; got.Offset != 990 || got.Limit != 10 {
		t.Errorf("straddling query = %+v, want Offset=990 Limit=10", got)
	}
}
