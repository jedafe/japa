package publicapi

// Phase 3 tests (spec 2026-09-24-agent-data-api-phase3): admin-only scope
// gating, key-scoped bookmark/draft/webhook handlers.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Scope gating
// ---------------------------------------------------------------------------

func TestSelfServiceScopesExcludeAdminOnly(t *testing.T) {
	for _, s := range SelfServiceScopes() {
		if s.AdminOnly {
			t.Errorf("admin-only scope %q leaked into self-service catalog", s.Value)
		}
		if strings.HasPrefix(s.Value, "api.write.") || s.Value == "*" {
			t.Errorf("write/wildcard scope %q is self-service — must be admin-only", s.Value)
		}
	}
}

func TestAdminOnlyScopeMarks(t *testing.T) {
	want := map[string]bool{
		"api.write.content":   true,
		"api.write.bookmarks": true,
		"api.write.comments":  true,
		"api.write.me":        true,
		"agent.write":         true,
		"*":                   true,
		"agent.use":           false,
		"api.read.content":    false,
		"api.read.me":         false,
		"api.read.*":          false,
	}
	byValue := make(map[string]ScopeOption, len(ScopeCatalog))
	for _, s := range ScopeCatalog {
		byValue[s.Value] = s
	}
	for value, wantAdmin := range want {
		s, ok := byValue[value]
		if !ok {
			t.Fatalf("scope %q missing from catalog", value)
		}
		if s.AdminOnly != wantAdmin {
			t.Errorf("scope %q AdminOnly=%v, want %v", value, s.AdminOnly, wantAdmin)
		}
	}
}

func TestAnyAdminOnly(t *testing.T) {
	for _, tc := range []struct {
		scopes []string
		want   bool
	}{
		{[]string{"api.read.content"}, false},
		{[]string{"api.read.*"}, false},
		{nil, false},
		{[]string{"api.read.content", "api.write.bookmarks"}, true},
		{[]string{"*"}, true},
		{[]string{"api.write.me"}, true},
		{[]string{"agent.write"}, true},
		// A requested wildcard that covers admin-only leaves is equally
		// privileged — it must require admin approval or the mint gate is
		// bypassed (HasScope matches "api.write.*" against api.write.content).
		{[]string{"api.write.*"}, true},
		{[]string{"agent.*"}, true},
		{[]string{"api.*"}, true},
		{[]string{"agent.use", "agent.*"}, true},
		// Wildcards that cover no admin-only leaf stay self-serve.
		{[]string{"api.read.*"}, false},
		{[]string{"bogus.*"}, false},
		{[]string{"bogus.scope"}, false}, // unknown scopes are inert, not admin-only
	} {
		if got := AnyAdminOnly(tc.scopes); got != tc.want {
			t.Errorf("AnyAdminOnly(%v) = %v, want %v", tc.scopes, got, tc.want)
		}
	}
}

type fakeAdminChecker struct{ allow bool }

func (f fakeAdminChecker) HasPermission(context.Context, pgtype.UUID, string) bool { return f.allow }

// The scope gate runs before any store access, so a nil store is fine —
// rejection must never reach the DB.
func TestCreateAPIKeyScopeGating(t *testing.T) {
	owner := pgtype.UUID{Valid: true}
	owner.Bytes[0] = 1

	writeScopeInput := CreateAPIKeyInput{Name: "k", Scopes: []string{"api.write.bookmarks"}}
	readScopeInput := CreateAPIKeyInput{Name: "k", Scopes: []string{"api.read.content"}}

	// No checker wired → fail closed on admin-only scopes.
	svc := NewService(nil)
	if _, err := svc.CreateAPIKey(context.Background(), owner, writeScopeInput); !errors.Is(err, ErrScopeRequiresAdmin) {
		t.Errorf("nil checker + write scope: got %v, want ErrScopeRequiresAdmin", err)
	}
	if _, err := svc.CreateAPIKey(context.Background(), owner, CreateAPIKeyInput{Name: "k", Scopes: []string{"*"}}); !errors.Is(err, ErrScopeRequiresAdmin) {
		t.Errorf("nil checker + wildcard: got %v, want ErrScopeRequiresAdmin", err)
	}

	// Non-admin checker → rejected.
	svc = NewService(nil)
	svc.SetAdminChecker(fakeAdminChecker{allow: false})
	if _, err := svc.CreateAPIKey(context.Background(), owner, writeScopeInput); !errors.Is(err, ErrScopeRequiresAdmin) {
		t.Errorf("non-admin + write scope: got %v, want ErrScopeRequiresAdmin", err)
	}

	// Non-admin minting for another user is itself gated — even with
	// read-only scopes.
	other := pgtype.UUID{Valid: true}
	other.Bytes[0] = 2
	if _, err := svc.CreateAPIKeyForUser(context.Background(), owner, other, readScopeInput); !errors.Is(err, ErrScopeRequiresAdmin) {
		t.Errorf("non-admin cross-user mint: got %v, want ErrScopeRequiresAdmin", err)
	}
}

// ---------------------------------------------------------------------------
// Bookmark handlers
// ---------------------------------------------------------------------------

type stubBookmarkService struct {
	listFn   func(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]PublicBookmarkItem, error)
	addFn    func(ctx context.Context, userID, contentID, folderID pgtype.UUID) (PublicBookmarkItem, error)
	removeFn func(ctx context.Context, userID, contentID pgtype.UUID) error
}

func (s stubBookmarkService) ListBookmarks(ctx context.Context, u pgtype.UUID, l, o int32) ([]PublicBookmarkItem, error) {
	if s.listFn == nil {
		return nil, nil
	}
	return s.listFn(ctx, u, l, o)
}
func (s stubBookmarkService) AddContentBookmark(ctx context.Context, u, c, f pgtype.UUID) (PublicBookmarkItem, error) {
	return s.addFn(ctx, u, c, f)
}
func (s stubBookmarkService) RemoveContentBookmark(ctx context.Context, u, c pgtype.UUID) error {
	return s.removeFn(ctx, u, c)
}

var testContentID = pgtype.UUID{Valid: true, Bytes: [16]byte{9}}

func TestBookmarkHandlersRequireKey(t *testing.T) {
	h := NewHandlers(NewService(nil))
	h.SetBookmarkService(stubBookmarkService{})

	rec := httptest.NewRecorder()
	h.ListMyBookmarks(rec, httptest.NewRequest(http.MethodGet, "/public/me/bookmarks", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("list without key: got %d, want 401", rec.Code)
	}
}

func TestPutMyBookmark(t *testing.T) {
	userID := pgtype.UUID{Valid: true, Bytes: [16]byte{7}}
	h := NewHandlers(NewService(nil))
	h.SetBookmarkService(stubBookmarkService{
		addFn: func(_ context.Context, u, c, f pgtype.UUID) (PublicBookmarkItem, error) {
			if u != userID || c != testContentID {
				t.Fatalf("add called with wrong owner/content: %v %v", u, c)
			}
			return PublicBookmarkItem{ID: "bm-1", EntityType: "content", EntityID: c.String()}, nil
		},
	})

	req := withChiParams(httptest.NewRequest(http.MethodPut, "/public/me/bookmarks/x", nil), map[string]string{"content_id": testContentID.String()})
	rec := httptest.NewRecorder()
	h.PutMyBookmark(rec, withAPIKeyUser(req, userID))
	if rec.Code != http.StatusCreated {
		t.Fatalf("put: got %d, want 201 — %s", rec.Code, rec.Body.String())
	}
}

func TestPutMyBookmarkInvisibleContent404(t *testing.T) {
	h := NewHandlers(NewService(nil))
	h.SetBookmarkService(stubBookmarkService{
		addFn: func(context.Context, pgtype.UUID, pgtype.UUID, pgtype.UUID) (PublicBookmarkItem, error) {
			return PublicBookmarkItem{}, errpkg.ErrNotFound
		},
	})

	req := withChiParams(httptest.NewRequest(http.MethodPut, "/public/me/bookmarks/x", nil), map[string]string{"content_id": testContentID.String()})
	rec := httptest.NewRecorder()
	h.PutMyBookmark(rec, withAPIKeyUser(req, pgtype.UUID{Valid: true}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("invisible content: got %d, want 404 — %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteMyBookmarkNotFound(t *testing.T) {
	h := NewHandlers(NewService(nil))
	h.SetBookmarkService(stubBookmarkService{
		removeFn: func(context.Context, pgtype.UUID, pgtype.UUID) error {
			return errpkg.ErrNotFound
		},
	})

	req := withChiParams(httptest.NewRequest(http.MethodDelete, "/public/me/bookmarks/x", nil), map[string]string{"content_id": testContentID.String()})
	rec := httptest.NewRecorder()
	h.DeleteMyBookmark(rec, withAPIKeyUser(req, pgtype.UUID{Valid: true}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing bookmark: got %d, want 404", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Draft handler
// ---------------------------------------------------------------------------

type stubDraftService struct {
	createFn func(ctx context.Context, ownerID pgtype.UUID, in CreateDraftInput) (PublicDraft, error)
}

func (s stubDraftService) CreateDraft(ctx context.Context, o pgtype.UUID, in CreateDraftInput) (PublicDraft, error) {
	return s.createFn(ctx, o, in)
}

func draftRequest(t *testing.T, body string, idemKey bool) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/public/content", strings.NewReader(body))
	if idemKey {
		req.Header.Set("Idempotency-Key", "test-key-1")
	}
	return withAPIKeyUser(req, pgtype.UUID{Valid: true})
}

func TestCreatePublicDraftRequiresIdempotencyKey(t *testing.T) {
	h := NewHandlers(NewService(nil))
	h.SetContentDraftService(stubDraftService{createFn: func(context.Context, pgtype.UUID, CreateDraftInput) (PublicDraft, error) {
		return PublicDraft{Status: "draft"}, nil
	}})

	rec := httptest.NewRecorder()
	h.CreatePublicDraft(rec, draftRequest(t, `{"title":"t","body":"b"}`, false))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing Idempotency-Key: got %d, want 400", rec.Code)
	}
}

func TestCreatePublicDraftRejectsPublishedStatus(t *testing.T) {
	h := NewHandlers(NewService(nil))
	h.SetContentDraftService(stubDraftService{createFn: func(context.Context, pgtype.UUID, CreateDraftInput) (PublicDraft, error) {
		t.Fatal("service must not be called for a non-draft status")
		return PublicDraft{}, nil
	}})

	rec := httptest.NewRecorder()
	h.CreatePublicDraft(rec, draftRequest(t, `{"title":"t","body":"b","status":"published"}`, true))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=published: got %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status_not_allowed") {
		t.Fatalf("status=published: want code status_not_allowed, got %s", rec.Body.String())
	}
}

func TestCreatePublicDraftContentTypeAllowlist(t *testing.T) {
	h := NewHandlers(NewService(nil))
	h.SetContentDraftService(stubDraftService{createFn: func(context.Context, pgtype.UUID, CreateDraftInput) (PublicDraft, error) {
		return PublicDraft{Status: "draft"}, nil
	}})

	for _, tc := range []struct {
		ctype string
		want  int
	}{
		{"blog_post", http.StatusCreated},
		{"guide", http.StatusCreated},
		{"story", http.StatusCreated},
		{"discussion", http.StatusCreated},
		{"video", http.StatusBadRequest},
		{"service_listing", http.StatusBadRequest},
	} {
		rec := httptest.NewRecorder()
		h.CreatePublicDraft(rec, draftRequest(t, `{"title":"t","body":"b","content_type":"`+tc.ctype+`"}`, true))
		if rec.Code != tc.want {
			t.Errorf("content_type=%q: got %d, want %d", tc.ctype, rec.Code, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Webhook subscription handlers
// ---------------------------------------------------------------------------

type stubWebhookService struct {
	count    int64
	createFn func(ctx context.Context, ownerID pgtype.UUID, name, url string, eventTypes []string) (PublicWebhookEndpoint, error)
}

func (s stubWebhookService) CreateUserEndpoint(ctx context.Context, o pgtype.UUID, n, u string, et []string) (PublicWebhookEndpoint, error) {
	return s.createFn(ctx, o, n, u, et)
}
func (s stubWebhookService) ListUserEndpoints(context.Context, pgtype.UUID) ([]PublicWebhookEndpoint, error) {
	return nil, nil
}
func (s stubWebhookService) CountUserEndpoints(context.Context, pgtype.UUID) (int64, error) {
	return s.count, nil
}
func (s stubWebhookService) VerifyUserEndpoint(context.Context, pgtype.UUID, pgtype.UUID) error {
	return nil
}
func (s stubWebhookService) DeleteUserEndpoint(context.Context, pgtype.UUID, pgtype.UUID) error {
	return nil
}
func (s stubWebhookService) ListUserDeliveries(context.Context, pgtype.UUID, pgtype.UUID, int32, int32) ([]PublicWebhookDelivery, error) {
	return nil, nil
}

func webhookRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	return withAPIKeyUser(httptest.NewRequest(http.MethodPost, "/public/me/webhooks", strings.NewReader(body)), pgtype.UUID{Valid: true})
}

func TestCreateMyWebhookEventTypeRestriction(t *testing.T) {
	h := NewHandlers(NewService(nil))
	h.SetWebhookSubscriptions(stubWebhookService{createFn: func(context.Context, pgtype.UUID, string, string, []string) (PublicWebhookEndpoint, error) {
		return PublicWebhookEndpoint{ID: "ep-1"}, nil
	}})

	// Non-allowlisted event type → 400.
	rec := httptest.NewRecorder()
	h.CreateMyWebhook(rec, webhookRequest(t, `{"url":"https://example.com/hook","event_types":["payment.completed"]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad event type: got %d, want 400 — %s", rec.Code, rec.Body.String())
	}

	// Default (omitted) → content.published, succeeds.
	rec = httptest.NewRecorder()
	h.CreateMyWebhook(rec, webhookRequest(t, `{"url":"https://example.com/hook"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("default event type: got %d, want 201 — %s", rec.Code, rec.Body.String())
	}
}

func TestCreateMyWebhookMaxPerKey(t *testing.T) {
	h := NewHandlers(NewService(nil)) // no settings reader → fallback 5
	h.SetWebhookSubscriptions(stubWebhookService{
		count: 5,
		createFn: func(context.Context, pgtype.UUID, string, string, []string) (PublicWebhookEndpoint, error) {
			t.Fatal("create must not run at the cap")
			return PublicWebhookEndpoint{}, nil
		},
	})

	rec := httptest.NewRecorder()
	h.CreateMyWebhook(rec, webhookRequest(t, `{"url":"https://example.com/hook"}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("at cap: got %d, want 403 — %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "webhook_limit_reached") {
		t.Fatalf("at cap: want code webhook_limit_reached, got %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Comment handler
// ---------------------------------------------------------------------------

type stubCommentService struct {
	createFn func(ctx context.Context, userID, contentID, parentID pgtype.UUID, body, clientIP string) (PublicComment, error)
}

func (s stubCommentService) CreateComment(ctx context.Context, u, c, p pgtype.UUID, b, ip string) (PublicComment, error) {
	return s.createFn(ctx, u, c, p, b, ip)
}

func commentRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/public/content/x/comments", strings.NewReader(body))
	req = withChiParams(req, map[string]string{"id": testContentID.String()})
	return withAPIKeyUser(req, pgtype.UUID{Valid: true})
}

func TestCreatePublicCommentValidation(t *testing.T) {
	h := NewHandlers(NewService(nil))
	h.SetCommentService(stubCommentService{createFn: func(context.Context, pgtype.UUID, pgtype.UUID, pgtype.UUID, string, string) (PublicComment, error) {
		return PublicComment{ID: "c1", Depth: 0}, nil
	}})

	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"missing body", `{"body":"  "}`, http.StatusBadRequest},
		{"anonymous rejected", `{"body":"hi","is_anonymous":true}`, http.StatusBadRequest},
		{"bad parent_id", `{"body":"hi","parent_id":"nope"}`, http.StatusBadRequest},
		{"ok", `{"body":"hi"}`, http.StatusCreated},
		{"reply ok", `{"body":"hi","parent_id":"` + testContentID.String() + `"}`, http.StatusCreated},
	} {
		rec := httptest.NewRecorder()
		h.CreatePublicComment(rec, commentRequest(t, tc.body))
		if rec.Code != tc.want {
			t.Errorf("%s: got %d, want %d — %s", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}
}

func TestCreatePublicCommentPrivacyAndBlocking(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
		code string
	}{
		{"invisible content → 404", errpkg.ErrNotFound, http.StatusNotFound, "not_found"},
		{"spam blocked → 403", ErrContentBlocked, http.StatusForbidden, "content_blocked"},
	} {
		h := NewHandlers(NewService(nil))
		h.SetCommentService(stubCommentService{createFn: func(context.Context, pgtype.UUID, pgtype.UUID, pgtype.UUID, string, string) (PublicComment, error) {
			return PublicComment{}, tc.err
		}})
		rec := httptest.NewRecorder()
		h.CreatePublicComment(rec, commentRequest(t, `{"body":"hi"}`))
		if rec.Code != tc.want {
			t.Errorf("%s: got %d, want %d — %s", tc.name, rec.Code, tc.want, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), tc.code) {
			t.Errorf("%s: want code %q, got %s", tc.name, tc.code, rec.Body.String())
		}
	}
}

// ---------------------------------------------------------------------------
// OpenAPI surface
// ---------------------------------------------------------------------------

func TestPhase3PathsDocumented(t *testing.T) {
	spec := GetPublicOpenAPISpec("https://api.test", "Test")
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("spec paths missing")
	}
	for _, p := range []string{
		"/public/me/bookmarks",
		"/public/me/bookmarks/{content_id}",
		"/public/me/webhooks",
		"/public/me/webhooks/{id}",
		"/public/me/webhooks/{id}/verify",
		"/public/me/webhooks/{id}/deliveries",
	} {
		if _, ok := paths[p]; !ok {
			t.Errorf("path %s missing from OpenAPI spec", p)
		}
	}
	// POST /public/content must sit alongside GET on the same path entry.
	content, ok := paths["/public/content"].(map[string]any)
	if !ok {
		t.Fatal("/public/content missing")
	}
	if _, ok := content["post"]; !ok {
		t.Error("POST /public/content not documented")
	}
}
