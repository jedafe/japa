package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	"github.com/jedafe/japa/internal/auth"
)

// Fixed UUIDs for deterministic test assertions.
const (
	testUser1 = "550e8400-e29b-41d4-a716-446655440001"
	testUser2 = "550e8400-e29b-41d4-a716-446655440002"
)

// editLockTestRedis spins up an in-memory Valkey for edit lock tests.
func editLockTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return mr, rdb
}

// editLockRequest builds an *http.Request with the given method, path,
// and an optional authenticated user in context. When userID is empty the
// request is anonymous. userID must be a valid UUID string.
func editLockRequest(method, path, userID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if userID != "" {
		var uid pgtype.UUID
		if err := uid.Scan(userID); err != nil {
			panic("editLockRequest: invalid userID: " + userID)
		}
		user := &auth.AuthenticatedUser{UserID: uid}
		ctx := context.WithValue(req.Context(), auth.UserContextKey, user)
		req = req.WithContext(ctx)
	}
	return req
}

// editLockResponse parses the JSON body of a response recorder.
func editLockResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	return body
}

// ---------------------------------------------------------------------------
// Basic acquire / refresh / conflict
// ---------------------------------------------------------------------------

func TestEditLock_AcquiresOnPost(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodPost, "/api/v1/posts/abc-123", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	val, err := rdb.Get(context.Background(), "jc:editor_lock:posts:abc-123").Result()
	if err != nil {
		t.Fatalf("lock not set: %v", err)
	}
	if val != testUser1 {
		t.Fatalf("lock value mismatch: got %q, want %q", val, testUser1)
	}
}

func TestEditLock_AcquiresOnPut(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodPut, "/api/v1/posts/def-456", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	val, err := rdb.Get(context.Background(), "jc:editor_lock:posts:def-456").Result()
	if err != nil {
		t.Fatalf("lock not set: %v", err)
	}
	if val != testUser1 {
		t.Fatalf("lock value mismatch: got %q, want %q", val, testUser1)
	}
}

func TestEditLock_AcquiresOnPatch(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodPatch, "/api/v1/posts/ghi-789", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	val, err := rdb.Get(context.Background(), "jc:editor_lock:posts:ghi-789").Result()
	if err != nil {
		t.Fatalf("lock not set: %v", err)
	}
	if val != testUser1 {
		t.Fatalf("lock value mismatch: got %q, want %q", val, testUser1)
	}
}

func TestEditLock_ReleasesOnDelete(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	// Acquire the lock first.
	rdb.Set(context.Background(), "jc:editor_lock:posts:abc-123", testUser1, editLockTTL)

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodDelete, "/api/v1/posts/abc-123", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	_, err := rdb.Get(context.Background(), "jc:editor_lock:posts:abc-123").Result()
	if err != redis.Nil {
		t.Fatalf("lock should have been deleted, err=%v", err)
	}
}

func TestEditLock_SameUserCanRefreshLock(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	// User acquires the lock.
	rr1 := httptest.NewRecorder()
	mw(h).ServeHTTP(rr1, editLockRequest(http.MethodPost, "/api/v1/posts/abc-123", testUser1))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr1.Code)
	}

	// Same user edits again — should refresh without conflict.
	rr2 := httptest.NewRecorder()
	mw(h).ServeHTTP(rr2, editLockRequest(http.MethodPut, "/api/v1/posts/abc-123", testUser1))
	if rr2.Code != http.StatusOK {
		t.Fatalf("second request: expected 200, got %d", rr2.Code)
	}
}

func TestEditLock_DifferentUserGetsConflict(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	// User 1 acquires the lock.
	rr1 := httptest.NewRecorder()
	mw(h).ServeHTTP(rr1, editLockRequest(http.MethodPost, "/api/v1/posts/abc-123", testUser1))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr1.Code)
	}

	// User 2 tries to edit — should get 409.
	rr2 := httptest.NewRecorder()
	mw(h).ServeHTTP(rr2, editLockRequest(http.MethodPut, "/api/v1/posts/abc-123", testUser2))
	if rr2.Code != http.StatusConflict {
		t.Fatalf("second request: expected 409, got %d", rr2.Code)
	}

	body := editLockResponse(t, rr2)
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object in response")
	}
	if errObj["code"] != "edit_conflict" {
		t.Fatalf("expected edit_conflict, got %v", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// Safe methods pass through
// ---------------------------------------------------------------------------

func TestEditLock_PassesThroughOnGET(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodGet, "/api/v1/posts/abc-123", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	keys, _ := rdb.Keys(context.Background(), "jc:editor_lock:*").Result()
	if len(keys) > 0 {
		t.Fatalf("expected no lock keys, got %v", keys)
	}
}

func TestEditLock_PassesThroughOnHEAD(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodHead, "/api/v1/posts/abc-123", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Unauthenticated requests
// ---------------------------------------------------------------------------

func TestEditLock_AnonymousUserPassesThrough(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodPost, "/api/v1/posts/abc-123", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	keys, _ := rdb.Keys(context.Background(), "jc:editor_lock:*").Result()
	if len(keys) > 0 {
		t.Fatalf("expected no lock keys for anonymous, got %v", keys)
	}
}

// ---------------------------------------------------------------------------
// Graceful degradation
// ---------------------------------------------------------------------------

func TestEditLock_NilRedisDegradesGracefully(t *testing.T) {
	mw := EditLockMiddleware(nil)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodPost, "/api/v1/posts/abc-123", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestEditLock_ValkeyUnreachableDegradesGracefully(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer rdb.Close()

	mw := EditLockMiddleware(rdb)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodPost, "/api/v1/posts/abc-123", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Path parsing
// ---------------------------------------------------------------------------

func TestEditLock_ParsesStandardRESTPath(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodPost, "/api/v1/comments/comment-42", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	val, err := rdb.Get(context.Background(), "jc:editor_lock:comments:comment-42").Result()
	if err != nil {
		t.Fatalf("lock not set: %v", err)
	}
	if val != testUser1 {
		t.Fatalf("lock value mismatch: got %q, want %q", val, testUser1)
	}
}

func TestEditLock_ParsesNestedResourcePath(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodPost, "/api/v1/posts/123/comments/456", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	val, err := rdb.Get(context.Background(), "jc:editor_lock:comments:456").Result()
	if err != nil {
		t.Fatalf("lock not set: %v", err)
	}
	if val != testUser1 {
		t.Fatalf("lock value mismatch: got %q, want %q", val, testUser1)
	}
}

func TestEditLock_ShortPathPassesThrough(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	// Two-segment path — not enough info to distinguish resource from
	// collection, so the middleware passes through without locking.
	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodPost, "/posts/abc", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	keys, _ := rdb.Keys(context.Background(), "jc:editor_lock:*").Result()
	if len(keys) > 0 {
		t.Fatalf("expected no lock keys for two-segment path, got %v", keys)
	}
}

func TestEditLock_TwoSegmentPathPassesThrough(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodPost, "/posts", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	keys, _ := rdb.Keys(context.Background(), "jc:editor_lock:*").Result()
	if len(keys) > 0 {
		t.Fatalf("expected no lock keys for single-segment path, got %v", keys)
	}
}

func TestEditLock_EmptyPathPassesThrough(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodPost, "/", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	keys, _ := rdb.Keys(context.Background(), "jc:editor_lock:*").Result()
	if len(keys) > 0 {
		t.Fatalf("expected no lock keys for empty path, got %v", keys)
	}
}

// ---------------------------------------------------------------------------
// extractContentFromPath unit tests
// ---------------------------------------------------------------------------

func TestExtractContentFromPath(t *testing.T) {
	tests := []struct {
		path     string
		wantType string
		wantID   string
	}{
		{"/api/v1/posts/abc-123", "posts", "abc-123"},
		{"/api/v1/posts/123/comments/456", "comments", "456"},
		{"/", "", ""},
		{"", "", ""},
		// Two segments — not enough to distinguish resource from collection.
		{"/api/v1/posts", "", ""},
		{"/posts/abc", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			ct, cid := extractContentFromPath(tt.path)
			if ct != tt.wantType || cid != tt.wantID {
				t.Errorf("extractContentFromPath(%q) = (%q, %q), want (%q, %q)",
					tt.path, ct, cid, tt.wantType, tt.wantID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DELETE ownership
// ---------------------------------------------------------------------------

func TestEditLock_DeleteOwnLockSucceeds(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rdb.Set(context.Background(), "jc:editor_lock:posts:abc-123", testUser1, editLockTTL)

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodDelete, "/api/v1/posts/abc-123", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	_, err := rdb.Get(context.Background(), "jc:editor_lock:posts:abc-123").Result()
	if err != redis.Nil {
		t.Fatalf("lock should have been deleted, err=%v", err)
	}
}

func TestEditLock_DeleteOtherUsersLockForbidden(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rdb.Set(context.Background(), "jc:editor_lock:posts:abc-123", testUser1, editLockTTL)

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodDelete, "/api/v1/posts/abc-123", testUser2))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}

	// Lock should still exist and belong to user-1.
	val, err := rdb.Get(context.Background(), "jc:editor_lock:posts:abc-123").Result()
	if err != nil {
		t.Fatalf("lock should still exist: %v", err)
	}
	if val != testUser1 {
		t.Fatalf("lock ownership changed: got %q, want %q", val, testUser1)
	}
}

func TestEditLock_DeleteNonexistentLockPassesThrough(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, editLockRequest(http.MethodDelete, "/api/v1/posts/abc-123", testUser1))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Conflict does not overwrite existing lock
// ---------------------------------------------------------------------------

func TestEditLock_ConflictDoesNotOverwriteExistingLock(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr1 := httptest.NewRecorder()
	mw(h).ServeHTTP(rr1, editLockRequest(http.MethodPost, "/api/v1/posts/abc-123", testUser1))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	mw(h).ServeHTTP(rr2, editLockRequest(http.MethodPut, "/api/v1/posts/abc-123", testUser2))
	if rr2.Code != http.StatusConflict {
		t.Fatalf("second request: expected 409, got %d", rr2.Code)
	}

	val, err := rdb.Get(context.Background(), "jc:editor_lock:posts:abc-123").Result()
	if err != nil {
		t.Fatalf("lock should still exist: %v", err)
	}
	if val != testUser1 {
		t.Fatalf("lock ownership changed: got %q, want %q", val, testUser1)
	}
}

func TestEditLock_ConflictResponseBody(t *testing.T) {
	_, rdb := editLockTestRedis(t)
	mw := EditLockMiddleware(rdb)
	h := handler()

	rr1 := httptest.NewRecorder()
	mw(h).ServeHTTP(rr1, editLockRequest(http.MethodPost, "/api/v1/posts/abc-123", testUser1))

	rr2 := httptest.NewRecorder()
	mw(h).ServeHTTP(rr2, editLockRequest(http.MethodPut, "/api/v1/posts/abc-123", testUser2))

	body := editLockResponse(t, rr2)
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object in response")
	}
	if errObj["code"] != "edit_conflict" {
		t.Fatalf("expected edit_conflict code, got %v", errObj["code"])
	}
	if errObj["message"] == "" {
		t.Fatalf("expected non-empty error message")
	}
}
