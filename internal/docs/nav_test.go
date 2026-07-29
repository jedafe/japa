package docs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scanning uuid %q: %v", s, err)
	}

	return id
}

func TestBuildDocNavTreeGroupsByAudience(t *testing.T) {
	t.Parallel()

	rows := []gen.ListPublishedDocPagesRow{
		{ID: mustUUID(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01"), Title: "Getting Started", Slug: "getting-started", Audience: "users", Position: 0},
		{ID: mustUUID(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02"), Title: "Authentication", Slug: "authentication", Audience: "api", Position: 0},
		{ID: mustUUID(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a03"), Title: "Profiles", Slug: "profiles", Audience: "users", Position: 1},
	}

	tree := buildDocNavTree(rows)

	if len(tree) != 2 {
		t.Fatalf("expected 2 audience groups, got %d", len(tree))
	}

	// Audience order follows the canonical ValidAudiences ordering: users before api.
	if tree[0].Audience != "users" || tree[1].Audience != "api" {
		t.Errorf("expected [users, api] group order, got [%s, %s]", tree[0].Audience, tree[1].Audience)
	}

	if len(tree[0].Items) != 2 {
		t.Errorf("expected 2 items in users group, got %d", len(tree[0].Items))
	}

	if tree[0].Items[0].Slug != "getting-started" {
		t.Errorf("expected first users item 'getting-started', got %q", tree[0].Items[0].Slug)
	}
}

func TestBuildDocNavTreeNestsChildren(t *testing.T) {
	t.Parallel()

	parentID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01"
	rows := []gen.ListPublishedDocPagesRow{
		{ID: mustUUID(t, parentID), Title: "Guide", Slug: "guide", Audience: "users", Position: 0},
		{ID: mustUUID(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02"), Title: "Step One", Slug: "step-one", Audience: "users", ParentID: mustUUID(t, parentID), Position: 0},
	}

	tree := buildDocNavTree(rows)

	if len(tree) != 1 || len(tree[0].Items) != 1 {
		t.Fatalf("expected 1 group with 1 root item, got %+v", tree)
	}

	root := tree[0].Items[0]
	if len(root.Children) != 1 || root.Children[0].Slug != "step-one" {
		t.Errorf("expected child 'step-one' under 'guide', got %+v", root.Children)
	}
}

func TestBuildDocNavTreeOrphanChildBecomesRoot(t *testing.T) {
	t.Parallel()

	rows := []gen.ListPublishedDocPagesRow{
		{ID: mustUUID(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02"), Title: "Orphan", Slug: "orphan", Audience: "users", ParentID: mustUUID(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a99"), Position: 0},
	}

	tree := buildDocNavTree(rows)

	if len(tree) != 1 || len(tree[0].Items) != 1 {
		t.Fatalf("expected orphan promoted to root, got %+v", tree)
	}

	if tree[0].Items[0].Slug != "orphan" {
		t.Errorf("expected orphan item, got %q", tree[0].Items[0].Slug)
	}
}

func TestBuildDocNavTreeEmpty(t *testing.T) {
	t.Parallel()

	tree := buildDocNavTree(nil)
	if len(tree) != 0 {
		t.Errorf("expected empty tree, got %+v", tree)
	}
}

func TestExtractExcerptTipTapContent(t *testing.T) {
	t.Parallel()

	content := []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello world."},{"type":"text","text":"Second sentence."}]}]}`)

	got := extractExcerpt(content, 140)
	want := "Hello world. Second sentence."

	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestExtractExcerptTruncates(t *testing.T) {
	t.Parallel()

	content := []byte(`{"type":"doc","content":[{"type":"text","text":"aaaaaaaaaabbbbbbbbbb"}]}`)

	got := extractExcerpt(content, 10)
	if len([]rune(got)) != 11 { // 10 chars + ellipsis
		t.Errorf("expected truncated excerpt of 11 runes, got %q (%d runes)", got, len([]rune(got)))
	}
}

func TestExtractExcerptInvalidJSON(t *testing.T) {
	t.Parallel()

	if got := extractExcerpt([]byte("not json"), 140); got != "" {
		t.Errorf("expected empty excerpt for invalid JSON, got %q", got)
	}
}

func TestSearchDocPagesHandlerRequiresQuery(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/docs/search", nil)
	ctx := context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: mustUUID(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h := &Handlers{Service: &Service{}}
	h.SearchDocPages(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing q, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "missing_query" {
		t.Errorf("expected error code 'missing_query', got %v", errObj["code"])
	}
}

// TestSearchDocPagesHandlerRejectsInvalidAudience verifies the audience query
// param is validated against ValidAudiences when provided (ADR-060 —
// audience-filtered search). Ref: sections/44-documentation-system.md
func TestSearchDocPagesHandlerRejectsInvalidAudience(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/docs/search?q=hello&audience=bogus", nil)
	ctx := context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: mustUUID(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h := &Handlers{Service: &Service{}}
	h.SearchDocPages(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid audience, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "invalid_audience" {
		t.Errorf("expected error code 'invalid_audience', got %v", errObj["code"])
	}
}
