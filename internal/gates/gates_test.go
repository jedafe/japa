package gates

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
)

const targetsJSON = `[
  {"key":"mail_admin","label":"Stalwart Admin","kind":"service","uuid":"svc-1","domain":"mail.japa.to"},
  {"key":"control","label":"Control Panel","kind":"application","uuid":"app-1","domain":"control.japa.to"},
  {"key":"admin","label":"Admin Panel","kind":"application","uuid":"app-2","domain":"admin.japa.to"},
  {"key":"control_api","label":"Control API","kind":"api_surface","prefix":"/control","flag":"feature.control_api_lockdown","domain":"api.japa.to"}
]`

// fakeFlags is an in-memory FlagIO for tests.
type fakeFlags struct {
	mu    sync.Mutex
	flags map[string]bool
	err   error
}

func (f *fakeFlags) IsEnabled(_ context.Context, key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.flags[key]
}

func (f *fakeFlags) SetEnabled(_ context.Context, key string, enabled bool, _ pgtype.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.flags[key] = enabled
	return nil
}

func testFlags() *fakeFlags { return &fakeFlags{flags: map[string]bool{}} }

// mockCoolify serves /api/v1/{applications|services}/{uuid}[/start|stop]
// from an in-memory status map.
type mockCoolify struct {
	mu       sync.Mutex
	statuses map[string]string // "kind/uuid" → status
	calls    []string          // recorded "METHOD path" for assertions
	token    string
}

func (m *mockCoolify) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+m.token {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		m.mu.Lock()
		m.calls = append(m.calls, r.Method+" "+r.URL.Path)
		defer m.mu.Unlock()

		var kind, uuid, action string
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
		if len(parts) >= 2 {
			kind, uuid = parts[0], parts[1]
		}
		if len(parts) == 3 {
			action = parts[2]
		}
		key := kind + "/" + uuid

		switch {
		case r.Method == http.MethodGet && action == "":
			st, ok := m.statuses[key]
			if !ok {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": st})
		case r.Method == http.MethodPost && (action == "start" || action == "stop"):
			if _, ok := m.statuses[key]; !ok {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			if action == "start" {
				m.statuses[key] = "running:unknown"
			} else {
				m.statuses[key] = "exited"
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Request accepted."})
		default:
			http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	return mux
}

func newTestService(t *testing.T, m *mockCoolify) (*Service, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(m.handler())
	t.Cleanup(srv.Close)
	svc, err := New(srv.URL, m.token, targetsJSON, testFlags())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc == nil {
		t.Fatal("New returned nil service for complete config")
	}
	return svc, srv
}

func TestNewDisabledWithoutTargets(t *testing.T) {
	for _, doc := range []string{"", "[]"} {
		svc, err := New("https://coolify", "tok", doc, testFlags())
		if err != nil {
			t.Fatalf("New(%q): unexpected error %v", doc, err)
		}
		if svc != nil {
			t.Fatalf("New(%q): expected nil service", doc)
		}
	}
	// Missing Coolify creds no longer disables the feature — Coolify-kind
	// targets report unknown; api_surface still works.
	if svc, err := New("", "", targetsJSON, testFlags()); err != nil || svc == nil {
		t.Fatalf("mixed config without coolify creds: svc=%v err=%v", svc, err)
	}
}

func TestNewValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
	}{
		{"bad json", `[{"key":`},
		{"missing uuid", `[{"key":"a","kind":"application"}]`},
		{"bad kind", `[{"key":"a","kind":"server","uuid":"u"}]`},
		{"duplicate key", `[{"key":"a","kind":"application","uuid":"u1"},{"key":"a","kind":"service","uuid":"u2"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New("https://coolify", "tok", tc.doc, testFlags()); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestListMapsStatuses(t *testing.T) {
	m := &mockCoolify{
		token: "tok",
		statuses: map[string]string{
			"services/svc-1":     "running:unknown",
			"applications/app-1": "exited",
			"applications/app-2": "restarting",
		},
	}
	svc, _ := newTestService(t, m)
	got := svc.List(context.Background())
	want := map[string]string{"mail_admin": "on", "control": "off", "admin": "transitioning", "control_api": "on"}
	for _, st := range got {
		if want[st.Key] != st.State {
			t.Fatalf("gate %q: state %q, want %q", st.Key, st.State, want[st.Key])
		}
	}
}

func TestListUnknownOnBackendError(t *testing.T) {
	m := &mockCoolify{token: "tok", statuses: map[string]string{}}
	svc, _ := newTestService(t, m)
	got := svc.List(context.Background())
	for _, st := range got {
		want := "unknown"
		if st.Key == "control_api" {
			want = "on" // flag-driven, not dependent on Coolify
		}
		if st.State != want {
			t.Fatalf("gate %q: expected %q, got %q", st.Key, want, st.State)
		}
	}
}

func TestToggleStopsRunningTarget(t *testing.T) {
	m := &mockCoolify{token: "tok", statuses: map[string]string{"services/svc-1": "running:healthy"}}
	svc, _ := newTestService(t, m)
	st, err := svc.Toggle(context.Background(), "mail_admin", pgtype.UUID{})
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if st.State != "off" {
		t.Fatalf("expected off, got %q", st.State)
	}
	var sawStop bool
	for _, c := range m.calls {
		if c == "POST /api/v1/services/svc-1/stop" {
			sawStop = true
		}
	}
	if !sawStop {
		t.Fatalf("stop not called; calls: %v", m.calls)
	}
}

func TestToggleStartsStoppedTarget(t *testing.T) {
	m := &mockCoolify{token: "tok", statuses: map[string]string{"applications/app-1": "exited"}}
	svc, _ := newTestService(t, m)
	st, err := svc.Toggle(context.Background(), "control", pgtype.UUID{})
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if st.State != "on" {
		t.Fatalf("expected on, got %q", st.State)
	}
	var sawStart bool
	for _, c := range m.calls {
		if c == "POST /api/v1/applications/app-1/start" {
			sawStart = true
		}
	}
	if !sawStart {
		t.Fatalf("start not called; calls: %v", m.calls)
	}
}

func TestToggleUnknownKey(t *testing.T) {
	m := &mockCoolify{token: "tok", statuses: map[string]string{}}
	svc, _ := newTestService(t, m)
	_, err := svc.Toggle(context.Background(), "nope", pgtype.UUID{})
	if !IsUnknownKey(err) {
		t.Fatalf("expected unknown-key error, got %v", err)
	}
}

func TestToggleSerialized(t *testing.T) {
	m := &mockCoolify{token: "tok", statuses: map[string]string{"services/svc-1": "running"}}
	svc, _ := newTestService(t, m)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = svc.Toggle(context.Background(), "mail_admin", pgtype.UUID{}) }()
	}
	wg.Wait()
	// Serialized mutex means GET→POST pairs never interleave: count
	// POST calls — each toggle issues exactly one start or stop.
	var posts int
	for _, c := range m.calls {
		if strings.HasPrefix(c, "POST ") {
			posts++
		}
	}
	if posts != 4 {
		t.Fatalf("expected 4 lifecycle calls, got %d (%v)", posts, m.calls)
	}
}

// --- handlers ---

type fakeRBAC struct{ super map[string]bool }

func (f fakeRBAC) IsSuperAdmin(_ context.Context, id pgtype.UUID) bool {
	return f.super[id.String()]
}

func userCtx(uid string) context.Context {
	id := pgtype.UUID{}
	// deterministic UUID bytes from the uid string hash — valid flag required
	copy(id.Bytes[:], []byte(uid + "................")[:16])
	id.Valid = true
	return context.WithValue(context.Background(), auth.UserContextKey, &auth.AuthenticatedUser{UserID: id})
}

func uidString(ctx context.Context) string {
	return auth.FromContext(ctx).UserID.String()
}

func callHandler(t *testing.T, h *Handlers, method, path, ctxUID string, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	// chi route context for {key} on toggle paths
	rctx := chi.NewRouteContext()
	if strings.HasSuffix(path, "/toggle") {
		parts := strings.Split(strings.TrimSuffix(path, "/toggle"), "/")
		rctx.URLParams.Add("key", parts[len(parts)-1])
	}
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	if method == http.MethodGet {
		h.HandleList(rr, req)
	} else {
		h.HandleToggle(rr, req)
	}
	return rr
}

func TestHandlersAuthZ(t *testing.T) {
	m := &mockCoolify{token: "tok", statuses: map[string]string{"services/svc-1": "running"}}
	svc, _ := newTestService(t, m)

	adminUID := "00000000-0000-0000-0000-0000000000aa"
	regularUID := "00000000-0000-0000-0000-0000000000bb"
	h := NewHandlers(svc, fakeRBAC{super: map[string]bool{}})

	// unauthenticated → 401
	if rr := callHandler(t, h, http.MethodGet, "/admin/access-gates", "", nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("no-auth: got %d", rr.Code)
	}
	// regular user → 403
	regularCtx := userCtx(regularUID)
	if rr := callHandler(t, h, http.MethodGet, "/admin/access-gates", regularUID, regularCtx); rr.Code != http.StatusForbidden {
		t.Fatalf("regular: got %d", rr.Code)
	}
	// super admin → 200
	rbac := fakeRBAC{super: map[string]bool{}}
	h = NewHandlers(svc, rbac)
	superCtx := userCtx(adminUID)
	rbac.super[uidString(superCtx)] = true
	rr := callHandler(t, h, http.MethodGet, "/admin/access-gates", adminUID, superCtx)
	if rr.Code != http.StatusOK {
		t.Fatalf("super: got %d body %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Gates []Status `json:"gates"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || len(body.Gates) != 4 {
		t.Fatalf("bad body: %v %s", err, rr.Body.String())
	}
	// super admin toggle → 200
	rr = callHandler(t, h, http.MethodPost, "/admin/access-gates/mail_admin/toggle", adminUID, superCtx)
	if rr.Code != http.StatusOK {
		t.Fatalf("toggle: got %d body %s", rr.Code, rr.Body.String())
	}
	// unknown gate → 404
	rr = callHandler(t, h, http.MethodPost, "/admin/access-gates/nope/toggle", adminUID, superCtx)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown gate: got %d", rr.Code)
	}
}

func TestHandlersDisabled(t *testing.T) {
	h := NewHandlers(nil, fakeRBAC{super: map[string]bool{}})
	// nil service must still be super-admin gated before feature_disabled
	regularCtx := userCtx("00000000-0000-0000-0000-0000000000cc")
	rr := callHandler(t, h, http.MethodGet, "/admin/access-gates", "", regularCtx)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("regular user on disabled feature: got %d, want 403", rr.Code)
	}
}

// --- api_surface (flag lockdown) ---

func TestToggleAPISurface(t *testing.T) {
	m := &mockCoolify{token: "tok", statuses: map[string]string{}}
	srv := httptest.NewServer(m.handler())
	t.Cleanup(srv.Close)
	flags := testFlags()
	svc, err := New(srv.URL, m.token, targetsJSON, flags)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// open → lock
	st, err := svc.Toggle(context.Background(), "control_api", pgtype.UUID{})
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if st.State != "off" || st.Raw != "locked" {
		t.Fatalf("expected off/locked, got %s/%s", st.State, st.Raw)
	}
	if !flags.flags["feature.control_api_lockdown"] {
		t.Fatal("flag not set")
	}
	// locked → unlock
	st, err = svc.Toggle(context.Background(), "control_api", pgtype.UUID{})
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if st.State != "on" || st.Raw != "open" {
		t.Fatalf("expected on/open, got %s/%s", st.State, st.Raw)
	}
}

func TestAPISurfaceListState(t *testing.T) {
	m := &mockCoolify{token: "tok", statuses: map[string]string{}}
	srv := httptest.NewServer(m.handler())
	t.Cleanup(srv.Close)
	flags := testFlags()
	flags.flags["feature.control_api_lockdown"] = true
	svc, _ := New(srv.URL, m.token, targetsJSON, flags)
	for _, st := range svc.List(context.Background()) {
		if st.Key == "control_api" && st.State != "off" {
			t.Fatalf("locked surface should read off, got %q", st.State)
		}
	}
}

func TestNewAPISurfaceRequiresFlags(t *testing.T) {
	if _, err := New("", "", `[{"key":"a","kind":"api_surface","prefix":"/x","flag":"f.x"}]`, nil); err == nil {
		t.Fatal("expected error when flag store missing")
	}
}

// --- probe ---

func TestProbe(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	t.Cleanup(up.Close)
	t.Cleanup(down.Close)
	svc := &Service{probe: up.Client(), probeCache: map[string]probeResult{}}
	if ok, st := svc.probeURL(context.Background(), up.URL); !ok || st != "200" {
		t.Fatalf("up: %v %q", ok, st)
	}
	if ok, st := svc.probeURL(context.Background(), down.URL); ok || st != "503" {
		t.Fatalf("down: %v %q", ok, st)
	}
	// unreachable host → timeout/error
	if ok, _ := svc.probeURL(context.Background(), "http://127.0.0.1:1/"); ok {
		t.Fatal("expected unreachable")
	}
}

// --- middleware ---

func TestRequireSurfaceOpen(t *testing.T) {
	flags := testFlags()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	mw := RequireSurfaceOpen(flags, "feature.control_api_lockdown")(next)

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, httptest.NewRequest("GET", "/control/x", nil))
	if rr.Code != 204 {
		t.Fatalf("open: got %d", rr.Code)
	}

	flags.flags["feature.control_api_lockdown"] = true
	rr = httptest.NewRecorder()
	mw.ServeHTTP(rr, httptest.NewRequest("GET", "/control/x", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("locked: got %d", rr.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.Error.Code != "surface_locked" {
		t.Fatalf("code: %s", body.Error.Code)
	}
}
