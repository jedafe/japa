package impersonation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// ---------------------------------------------------------------------------
// Context Helpers
// ---------------------------------------------------------------------------

func TestImpersonatedFromContext_NilWhenNotSet(t *testing.T) {
	t.Parallel()

	ic := ImpersonatedFromContext(t.Context())
	if ic != nil {
		t.Errorf("ImpersonatedFromContext() = %v, want nil", ic)
	}
}

func TestImpersonatedFromContext_ReturnsSetValue(t *testing.T) {
	t.Parallel()

	adminID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Valid: true}
	targetID := pgtype.UUID{Bytes: [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}, Valid: true}

	ic := &ImpersonationContext{
		AdminID:        adminID,
		ImpersonatedID: targetID,
	}
	ctx := context.WithValue(t.Context(), impersonatedUserKey, ic)

	got := ImpersonatedFromContext(ctx)
	if got == nil {
		t.Fatal("ImpersonatedFromContext() returned nil")
	}

	if got.AdminID != adminID {
		t.Errorf("AdminID = %v, want %v", got.AdminID, adminID)
	}

	if got.ImpersonatedID != targetID {
		t.Errorf("ImpersonatedID = %v, want %v", got.ImpersonatedID, targetID)
	}
}

func TestEffectiveUserID_ReturnsImpersonatedID(t *testing.T) {
	t.Parallel()

	adminID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	targetID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	ic := &ImpersonationContext{
		AdminID:        adminID,
		ImpersonatedID: targetID,
	}
	ctx := context.WithValue(t.Context(), impersonatedUserKey, ic)

	got := EffectiveUserID(ctx)
	if got != targetID {
		t.Errorf("EffectiveUserID() = %v, want %v", got, targetID)
	}
}

func TestEffectiveUserID_ReturnsZeroWhenNotImpersonating(t *testing.T) {
	t.Parallel()

	got := EffectiveUserID(t.Context())
	if got.Valid {
		t.Errorf("EffectiveUserID() = %v, want zero UUID", got)
	}
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestConstants(t *testing.T) {
	t.Parallel()

	if DefaultImpersonationTTL.Seconds() != 1800 {
		t.Errorf("DefaultImpersonationTTL = %v, want 30 minutes", DefaultImpersonationTTL)
	}

	if keyPrefix != "jc:impersonate:" {
		t.Errorf("keyPrefix = %q, want %q", keyPrefix, "jc:impersonate:")
	}
}

// ---------------------------------------------------------------------------
// Store — Valkey-Backed Operations
// ---------------------------------------------------------------------------

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey connect: %v", err)
	}

	compat := valkeycompat.NewAdapter(vk)

	t.Cleanup(func() { vk.Close() })

	return NewStore(compat), mr
}

func TestStoreSetAndGetImpersonation(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	adminID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	targetID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	err := store.SetImpersonation(t.Context(), adminID, targetID, DefaultImpersonationTTL)
	if err != nil {
		t.Fatalf("SetImpersonation() error: %v", err)
	}

	got, err := store.GetImpersonation(t.Context(), adminID)
	if err != nil {
		t.Fatalf("GetImpersonation() error: %v", err)
	}

	if got == "" {
		t.Fatal("GetImpersonation() returned empty string")
	}
}

func TestStoreGetImpersonationReturnsEmptyWhenNotSet(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	adminID := pgtype.UUID{Bytes: [16]byte{99}, Valid: true}

	got, err := store.GetImpersonation(t.Context(), adminID)
	if err != nil {
		t.Fatalf("GetImpersonation() error: %v", err)
	}

	if got != "" {
		t.Errorf("GetImpersonation() = %q, want empty", got)
	}
}

func TestStoreClearImpersonation(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	adminID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	targetID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	_ = store.SetImpersonation(t.Context(), adminID, targetID, DefaultImpersonationTTL)

	err := store.ClearImpersonation(t.Context(), adminID)
	if err != nil {
		t.Fatalf("ClearImpersonation() error: %v", err)
	}

	got, err := store.GetImpersonation(t.Context(), adminID)
	if err != nil {
		t.Fatalf("GetImpersonation() after clear error: %v", err)
	}

	if got != "" {
		t.Errorf("GetImpersonation() after clear = %q, want empty", got)
	}
}

func TestStoreClearImpersonationIdempotent(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	adminID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	// Clear when nothing set should not error
	err := store.ClearImpersonation(t.Context(), adminID)
	if err != nil {
		t.Fatalf("ClearImpersonation() on empty error: %v", err)
	}
}

func TestStoreNilRedisGracefulDegradation(t *testing.T) {
	t.Parallel()
	// Store with nil redis should degrade gracefully
	store := &Store{rdb: nil}
	adminID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	targetID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	err := store.SetImpersonation(t.Context(), adminID, targetID, DefaultImpersonationTTL)
	if err != nil {
		t.Errorf("SetImpersonation() with nil redis should return nil, got: %v", err)
	}

	got, err := store.GetImpersonation(t.Context(), adminID)
	if err != nil {
		t.Errorf("GetImpersonation() with nil redis should return nil, got: %v", err)
	}

	if got != "" {
		t.Errorf("GetImpersonation() with nil redis should return empty, got: %q", got)
	}

	err = store.ClearImpersonation(t.Context(), adminID)
	if err != nil {
		t.Errorf("ClearImpersonation() with nil redis should return nil, got: %v", err)
	}
}

func TestStoreTTLExpiry(t *testing.T) {
	t.Parallel()
	store, mr := newTestStore(t)
	adminID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	targetID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	_ = store.SetImpersonation(t.Context(), adminID, targetID, 10*time.Second)

	// Fast-forward time past the TTL
	mr.FastForward(11 * time.Second)

	got, err := store.GetImpersonation(t.Context(), adminID)
	if err != nil {
		t.Fatalf("GetImpersonation() error: %v", err)
	}

	if got != "" {
		t.Errorf("GetImpersonation() after TTL = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// Middleware — Context Injection
// ---------------------------------------------------------------------------

func TestImpersonationMiddleware_InjectsContext(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	adminID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	targetID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	// Set up impersonation state
	_ = store.SetImpersonation(t.Context(), adminID, targetID, DefaultImpersonationTTL)

	// Create a request with auth user in context
	var capturedIC *ImpersonationContext

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIC = ImpersonatedFromContext(r.Context())

		w.WriteHeader(http.StatusOK)
	})

	handler := ImpersonationMiddleware(store)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.AuthenticatedUser{
		UserID: adminID,
	})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handler status = %d, want %d", rec.Code, http.StatusOK)
	}

	if capturedIC == nil {
		t.Fatal("ImpersonatedFromContext() returned nil inside handler")
	}

	if capturedIC.AdminID != adminID {
		t.Errorf("AdminID = %v, want %v", capturedIC.AdminID, adminID)
	}

	if capturedIC.ImpersonatedID != targetID {
		t.Errorf("ImpersonatedID = %v, want %v", capturedIC.ImpersonatedID, targetID)
	}
}

func TestImpersonationMiddleware_NoImpersonationPassesThrough(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	adminID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	var capturedIC *ImpersonationContext

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIC = ImpersonatedFromContext(r.Context())

		w.WriteHeader(http.StatusOK)
	})

	handler := ImpersonationMiddleware(store)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.AuthenticatedUser{
		UserID: adminID,
	})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedIC != nil {
		t.Error("expected nil ImpersonationContext when no impersonation active")
	}
}

func TestImpersonationMiddleware_NoAuthUserPassesThrough(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)

	var called bool

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true

		w.WriteHeader(http.StatusOK)
	})

	handler := ImpersonationMiddleware(store)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No auth user in context
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("inner handler not called when no auth user")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestImpersonationMiddleware_NilRedisPassesThrough(t *testing.T) {
	t.Parallel()

	store := &Store{rdb: nil}
	adminID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	var capturedIC *ImpersonationContext

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIC = ImpersonatedFromContext(r.Context())

		w.WriteHeader(http.StatusOK)
	})

	handler := ImpersonationMiddleware(store)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.AuthenticatedUser{
		UserID: adminID,
	})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedIC != nil {
		t.Error("expected nil ImpersonationContext with nil redis store")
	}
}

func TestImpersonationMiddleware_InvalidUUIDPassesThrough(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	adminID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	// Set an invalid (non-UUID) value in redis
	_ = store.rdb.Set(t.Context(), keyPrefix+adminID.String(), "not-a-uuid", DefaultImpersonationTTL)

	var called bool

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true

		w.WriteHeader(http.StatusOK)
	})

	handler := ImpersonationMiddleware(store)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.AuthenticatedUser{
		UserID: adminID,
	})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("inner handler not called after invalid UUID scan")
	}
}

// ---------------------------------------------------------------------------
// Service — Start/End Impersonation (unit test with nil DB pool)
// ---------------------------------------------------------------------------

func TestServiceIsAdminWithoutRBACReturnsFalse(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	svc := NewService(store, nil)

	// Without an RBAC resolver wired, IsAdmin returns false (defensive).
	got := svc.IsAdmin(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, pgtype.UUID{Bytes: [16]byte{2}, Valid: true})
	if got {
		t.Error("IsAdmin() = true, want false when no RBAC resolver is wired")
	}
}

// stubAdminChecker is a test-only AdminCapabilityChecker.
type stubAdminChecker struct{ admin bool }

func (s stubAdminChecker) HasAdminCapability(_ context.Context, _, _ pgtype.UUID) bool {
	return s.admin
}

func TestServiceIsAdminWithRBACResolver(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	svc := NewService(store, nil)
	svc.SetRBAC(stubAdminChecker{admin: true})

	got := svc.IsAdmin(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, pgtype.UUID{Bytes: [16]byte{2}, Valid: true})
	if !got {
		t.Error("IsAdmin() = false, want true when resolver reports admin capability")
	}
}

func TestServiceGetImpersonationStatusNoActive(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	svc := NewService(store, nil)

	adminID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	targetID, active, _, err := svc.GetImpersonationStatus(t.Context(), adminID)
	if err != nil {
		t.Fatalf("GetImpersonationStatus() error: %v", err)
	}

	if active {
		t.Error("GetImpersonationStatus() active = true, want false")
	}

	if targetID != "" {
		t.Errorf("GetImpersonationStatus() targetID = %q, want empty", targetID)
	}
}

func TestServiceGetImpersonationStatusWithStore(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	svc := NewService(store, nil)

	adminID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	targetID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	// Directly set via store (service.StartImpersonation needs a DB pool for audit logs)
	_ = store.SetImpersonation(t.Context(), adminID, targetID, DefaultImpersonationTTL)

	gotTargetID, active, _, err := svc.GetImpersonationStatus(t.Context(), adminID)
	if err != nil {
		t.Fatalf("GetImpersonationStatus() error: %v", err)
	}

	if !active {
		t.Error("GetImpersonationStatus() active = false, want true")
	}

	if gotTargetID == "" {
		t.Error("GetImpersonationStatus() targetID is empty")
	}
}
