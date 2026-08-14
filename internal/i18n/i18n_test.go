package i18n

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Cache tests
// ---------------------------------------------------------------------------

func TestCacheSetAndGet(t *testing.T) {
	t.Parallel()

	svc := &Service{cache: make(map[cacheKey]*cacheEntry)}
	tenantID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	svc.cacheSet(tenantID, "en", map[string]string{"hello": "Hello"})

	m, ok := svc.cacheGet(tenantID, "en")
	if !ok {
		t.Fatal("expected cache hit")
	}

	if m["hello"] != "Hello" {
		t.Errorf("expected Hello, got %s", m["hello"])
	}
}

func TestCacheGetReturnsCopy(t *testing.T) {
	t.Parallel()

	svc := &Service{cache: make(map[cacheKey]*cacheEntry)}
	tenantID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	svc.cacheSet(tenantID, "en", map[string]string{"a": "1"})

	m, _ := svc.cacheGet(tenantID, "en")
	m["c"] = "3" // mutate the copy

	m2, _ := svc.cacheGet(tenantID, "en")
	if _, ok := m2["c"]; ok {
		t.Error("cache returned a shared reference instead of a copy")
	}
}

func TestCacheInvalidate(t *testing.T) {
	t.Parallel()

	svc := &Service{cache: make(map[cacheKey]*cacheEntry)}
	tenantID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	svc.cacheSet(tenantID, "en", map[string]string{"key": "val"})
	svc.cacheInvalidate(tenantID, "en")

	_, ok := svc.cacheGet(tenantID, "en")
	if ok {
		t.Error("expected cache miss after invalidation")
	}
}

func TestCacheMissForDifferentLocale(t *testing.T) {
	t.Parallel()

	svc := &Service{cache: make(map[cacheKey]*cacheEntry)}
	tenantID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	svc.cacheSet(tenantID, "en", map[string]string{"a": "1"})

	_, ok := svc.cacheGet(tenantID, "fr")
	if ok {
		t.Error("expected cache miss for different locale")
	}
}

func TestCacheMissForDifferentTenant(t *testing.T) {
	t.Parallel()

	svc := &Service{cache: make(map[cacheKey]*cacheEntry)}
	tenant1 := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	tenant2 := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	svc.cacheSet(tenant1, "en", map[string]string{"a": "1"})

	_, ok := svc.cacheGet(tenant2, "en")
	if ok {
		t.Error("expected cache miss for different tenant")
	}
}

// ---------------------------------------------------------------------------
// Handler helper tests
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	errpkg.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected ok, got %s", body["status"])
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	errpkg.WriteError(w, http.StatusBadRequest, "missing_locale", "locale is required")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var body map[string]map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body["error"]["code"] != "missing_locale" {
		t.Errorf("expected missing_locale, got %s", body["error"]["code"])
	}

	if body["error"]["message"] != "locale is required" {
		t.Errorf("expected 'locale is required', got %s", body["error"]["message"])
	}
}

// ---------------------------------------------------------------------------
// Handler context tests (no DB, just context guards)
// ---------------------------------------------------------------------------

func TestUpsertHandlerNoTenant(t *testing.T) {
	t.Parallel()

	svc := &Service{cache: make(map[cacheKey]*cacheEntry)}
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodPost, "/admin/translations", nil)
	w := httptest.NewRecorder()

	h.Upsert(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestGetLocaleTranslationsNoTenant(t *testing.T) {
	t.Parallel()

	svc := &Service{cache: make(map[cacheKey]*cacheEntry)}
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/translations/en", nil)
	w := httptest.NewRecorder()

	h.GetLocaleTranslations(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestListLocalesNoTenant(t *testing.T) {
	t.Parallel()

	svc := &Service{cache: make(map[cacheKey]*cacheEntry)}
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodGet, "/admin/translations/locales", nil)
	w := httptest.NewRecorder()

	h.ListLocales(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}
