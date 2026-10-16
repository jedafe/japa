package features

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
)

// flagReq builds a GET request carrying the given X-JC-Surface value (empty
// string = header absent), mirroring what the public flags endpoints see.
func flagReq(surface string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/feature-flags", nil)
	if surface != "" {
		r.Header.Set(SurfaceHeader, surface)
	}

	return r
}

// decodeData unmarshals the {"data": ...} response envelope into v.
func decodeData(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var env struct {
		Data json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	if err := json.Unmarshal(env.Data, v); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
}

// TestHandleGetFeatureFlags_ScopedMap verifies the public flags map is
// caller-scoped: a flag hidden on "main" reads false for main callers but
// true for other surfaces, and an audience-gated flag reads false for
// anonymous callers. The {data: map[string]bool} envelope is unchanged.
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §5.3.
func TestHandleGetFeatureFlags_ScopedMap(t *testing.T) {
	t.Parallel()

	h := NewHandlers(evalAllChecker(t))

	tests := []struct {
		name    string
		surface string
		key     string
		want    bool
	}{
		{"hidden on main reads false for main caller", "main", "feature.hiddenmain", false},
		{"audience-gated reads false for anonymous", "main", "feature.beta", false},
		{"globally off stays false", "main", "feature.off", false},
		{"plain flag on", "main", "feature.plain", true},
		{"registry default with no row", "main", "feature.blog", true},
		{"hidden rule is main-only — tools caller sees it", "tools", "feature.hiddenmain", true},
		{"beta rule is main-only — tools caller sees it", "tools", "feature.beta", true},
		{"no surface header applies no rules", "", "feature.hiddenmain", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			h.HandleGetFeatureFlags(rec, flagReq(tt.surface))

			var flags map[string]bool

			decodeData(t, rec, &flags)

			if got, ok := flags[tt.key]; !ok || got != tt.want {
				t.Errorf("flags[%q] = %v (present=%v), want %v", tt.key, got, ok, tt.want)
			}
		})
	}
}

// stagedRollout returns a partial rollout percentage guaranteed to keep
// "feature.staged" inside the rollout cohort: the deterministic bucket is
// fnvHash64(":"+key)%100 and a key is in-cohort when bucket < percentage,
// so bucket+1 always qualifies. Computing it keeps the test stable if the
// key name changes.
func stagedRollout() int {
	return int(fnvHash64(":feature.staged")%100) + 1
}

// detailsChecker seeds the aggregate with the shared eval rows plus a flag
// carrying explicit phase + rollout metadata.
func detailsChecker(t *testing.T) *Checker {
	t.Helper()

	c := newEvalTestChecker()
	c.cacheAllSet([]FlagRow{
		{
			Key:     "feature.staged",
			Enabled: true,
			Config: mustFlagConfig(t, FlagConfig{
				Phase:             "testing",
				RolloutPercentage: stagedRollout(),
			}),
		},
		{
			Key:     "feature.hiddenmain",
			Enabled: true,
			Config: surfacesConfig(t, map[string]SurfaceRule{
				"main": {State: "hidden"},
			}),
		},
		{
			Key:     "feature.beta",
			Enabled: true,
			Config: surfacesConfig(t, map[string]SurfaceRule{
				"main": {State: "visible", Audience: []string{"group:beta"}},
			}),
		},
		{Key: "feature.off", Enabled: false},
		{Key: "feature.plain", Enabled: true},
	})

	return c
}

// TestHandleGetFeatureFlagDetails verifies the public per-flag projection:
// registry + DB merge, caller-scoped enabled/visible, derived phase, and —
// critically — that surfaces/audience/geo rule fields never appear in the
// payload (internal config; spec §5.3 amended).
func TestHandleGetFeatureFlagDetails(t *testing.T) {
	t.Parallel()

	h := NewHandlers(detailsChecker(t))

	rec := httptest.NewRecorder()
	h.HandleGetFeatureFlagDetails(rec, flagReq("main"))

	body := rec.Body.String()

	for _, forbidden := range []string{`"surfaces"`, `"audience"`, `"geo_allow"`, `"geo_deny"`} {
		if strings.Contains(body, forbidden) {
			t.Errorf("response leaks internal config field %s: %s", forbidden, body)
		}
	}

	var details []FeatureFlagDetail

	decodeData(t, rec, &details)

	byKey := make(map[string]FeatureFlagDetail, len(details))
	for _, d := range details {
		byKey[d.Key] = d
	}

	tests := []struct {
		key     string
		enabled bool
		phase   string
		rollout int
		visible bool
	}{
		// Stored row with explicit phase + in-cohort rollout; enabled → visible.
		{"feature.staged", true, "testing", stagedRollout(), true},
		// Enabled but hidden on main → still listed, enabled, not visible.
		{"feature.hiddenmain", true, "shipped", 100, false},
		// Audience-gated flag reads fully off for anonymous callers.
		{"feature.beta", false, "shipped", 100, false},
		// Disabled flag derives the "planned" phase.
		{"feature.off", false, "planned", 100, false},
		// Registry seed with no stored row → default on, derived phase.
		{"feature.blog", true, "shipped", 100, true},
		// Registry seed default-off → planned.
		{"feature.wallet", false, "planned", 100, false},
	}

	for _, tt := range tests {
		d, ok := byKey[tt.key]
		if !ok {
			t.Errorf("key %q missing from details", tt.key)
			continue
		}

		if d.Enabled != tt.enabled || d.Phase != tt.phase || d.RolloutPercentage != tt.rollout || d.Visible != tt.visible {
			t.Errorf("%q = %+v, want enabled=%v phase=%q rollout=%d visible=%v",
				tt.key, d, tt.enabled, tt.phase, tt.rollout, tt.visible)
		}
	}
}

// TestHandleGetFeatureFlagDetails_OtherSurface verifies the details payload
// is caller-scoped: rules for "main" do not apply to a "tools" caller.
func TestHandleGetFeatureFlagDetails_OtherSurface(t *testing.T) {
	t.Parallel()

	h := NewHandlers(detailsChecker(t))

	rec := httptest.NewRecorder()
	h.HandleGetFeatureFlagDetails(rec, flagReq("tools"))

	var details []FeatureFlagDetail

	decodeData(t, rec, &details)

	byKey := make(map[string]FeatureFlagDetail, len(details))
	for _, d := range details {
		byKey[d.Key] = d
	}

	if d := byKey["feature.hiddenmain"]; !d.Enabled || !d.Visible {
		t.Errorf("feature.hiddenmain on tools = %+v, want enabled+visible (rule is main-only)", d)
	}

	if d := byKey["feature.beta"]; !d.Enabled || !d.Visible {
		t.Errorf("feature.beta on tools = %+v, want enabled+visible (rule is main-only)", d)
	}
}

// flagDetailReq builds a GET request for the single-key details endpoint,
// carrying the given X-JC-Surface value and the chi {key} URL param the
// router would populate.
func flagDetailReq(t *testing.T, surface, key string) *http.Request {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, "/api/v1/feature-flags/details/"+key, nil)
	if surface != "" {
		r.Header.Set(SurfaceHeader, surface)
	}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("key", key)

	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// TestHandleGetFeatureFlagDetail verifies the single-key form of the
// details projection: registry + stored-row merge, caller-scoped
// enabled/visible, derived phase — and the same internal-config embargo as
// the list endpoint (surfaces/audience/geo never appear in the payload).
func TestHandleGetFeatureFlagDetail(t *testing.T) {
	t.Parallel()

	h := NewHandlers(detailsChecker(t))

	tests := []struct {
		name    string
		surface string
		key     string
		enabled bool
		phase   string
		rollout int
		visible bool
	}{
		{"stored row with explicit phase + in-cohort rollout", "main", "feature.staged", true, "testing", stagedRollout(), true},
		{"hidden on main → enabled but not visible", "main", "feature.hiddenmain", true, "shipped", 100, false},
		{"audience-gated reads fully off for anonymous", "main", "feature.beta", false, "shipped", 100, false},
		{"disabled flag derives planned phase", "main", "feature.off", false, "planned", 100, false},
		{"registry seed with no stored row", "main", "feature.blog", true, "shipped", 100, true},
		{"registry default-off seed", "main", "feature.wallet", false, "planned", 100, false},
		{"main-only rule does not apply to tools caller", "tools", "feature.hiddenmain", true, "shipped", 100, true},
		{"no surface header applies no rules", "", "feature.hiddenmain", true, "shipped", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			h.HandleGetFeatureFlagDetail(rec, flagDetailReq(t, tt.surface, tt.key))

			body := rec.Body.String()

			for _, forbidden := range []string{`"surfaces"`, `"audience"`, `"geo_allow"`, `"geo_deny"`} {
				if strings.Contains(body, forbidden) {
					t.Errorf("response leaks internal config field %s: %s", forbidden, body)
				}
			}

			var d FeatureFlagDetail

			decodeData(t, rec, &d)

			if d.Key != tt.key || d.Enabled != tt.enabled || d.Phase != tt.phase ||
				d.RolloutPercentage != tt.rollout || d.Visible != tt.visible {
				t.Errorf("%q = %+v, want key=%q enabled=%v phase=%q rollout=%d visible=%v",
					tt.key, d, tt.key, tt.enabled, tt.phase, tt.rollout, tt.visible)
			}
		})
	}
}

// TestHandleGetFeatureFlagDetail_UnknownKey verifies a key that is neither a
// registry entry nor a stored feature_flags row 404s rather than returning
// a registry-default projection.
func TestHandleGetFeatureFlagDetail_UnknownKey(t *testing.T) {
	t.Parallel()

	h := NewHandlers(detailsChecker(t))

	rec := httptest.NewRecorder()
	h.HandleGetFeatureFlagDetail(rec, flagDetailReq(t, "main", "feature.nonexistent"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), `"not_found"`) {
		t.Errorf("404 body must carry not_found code, got %s", rec.Body.String())
	}
}

// TestGate_CallerScoped verifies the middleware evaluates the caller's full
// scope: a flag enabled globally but disabled on the caller's surface 404s
// with feature_disabled for that surface only; a globally disabled flag
// 404s everywhere; and anonymous requests (no auth/geo context) evaluate
// without error. Signature and 404 contract unchanged (ADR-042).
func TestGate_CallerScoped(t *testing.T) {
	t.Parallel()

	c := newEvalTestChecker()
	c.cacheSet("feature.test", true, surfacesConfig(t, map[string]SurfaceRule{
		"main": {State: "disabled"},
	}))
	c.cacheSet("feature.off", false, nil)

	tests := []struct {
		name       string
		key        string
		surface    string
		wantStatus int
		wantNext   bool
	}{
		{"disabled on caller surface → 404", "feature.test", "main", http.StatusNotFound, false},
		{"rule is main-only — tools passes", "feature.test", "tools", http.StatusNoContent, true},
		{"no surface → global behavior", "feature.test", "", http.StatusNoContent, true},
		{"unknown surface → global behavior", "feature.test", "web", http.StatusNoContent, true},
		{"globally off → 404 on any surface", "feature.off", "tools", http.StatusNotFound, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true

				w.WriteHeader(http.StatusNoContent)
			})

			rec := httptest.NewRecorder()
			Gate(c, tt.key)(next).ServeHTTP(rec, flagReq(tt.surface))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if nextCalled != tt.wantNext {
				t.Errorf("next called = %v, want %v", nextCalled, tt.wantNext)
			}

			if tt.wantStatus == http.StatusNotFound && !strings.Contains(rec.Body.String(), `"feature_disabled"`) {
				t.Errorf("404 body must carry feature_disabled code, got %s", rec.Body.String())
			}
		})
	}
}

// TestGate_ResolverUsed verifies Gate consults the wired AudienceResolver
// for authenticated requests: an audience-gated flag passes only when the
// resolver reports the caller inside the audience.
func TestGate_ResolverUsed(t *testing.T) {
	t.Parallel()

	c := newEvalTestChecker()
	c.cacheSet("feature.test", true, surfacesConfig(t, map[string]SurfaceRule{
		"main": {State: "visible", Audience: []string{"group:beta"}},
	}))

	resolver := &stubResolver{groups: map[string]bool{"beta": true}}
	c.SetAudienceResolver(resolver)

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Authenticated beta member on main → passes.
	r := scopeTestReq(true, false)
	r.Header.Set(SurfaceHeader, "main")

	rec := httptest.NewRecorder()
	Gate(c, "feature.test")(next).ServeHTTP(rec, r)

	if rec.Code != http.StatusNoContent {
		t.Errorf("beta member status = %d, want 204 (body: %s)", rec.Code, rec.Body.String())
	}

	// Same request without the resolver → anonymous → audience miss → 404.
	resolver.groups = nil
	rec = httptest.NewRecorder()
	Gate(c, "feature.test")(next).ServeHTTP(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Errorf("out-of-audience status = %d, want 404", rec.Code)
	}
}

// adminFlagWriteReq builds an authenticated admin write request carrying the
// chi flag_name URL param, mirroring what the admin router hands the flag
// write handlers.
func adminFlagWriteReq(t *testing.T, method, flagName string, body any) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}

	r := httptest.NewRequest(method, "/api/v1/admin/feature-flags/"+flagName, &buf)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("flag_name", flagName)

	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
	})

	return r.WithContext(ctx)
}

// TestFireConfigChanged verifies the checker-level hook: it fires
// fire-and-forget with the detached context (Rule 8) and the exact
// (key, cfg) payload handed to it — the payload DeleteFlag uses is an
// empty FlagConfig{} so consumers unlink the roadmap item (spec §4.3).
// DB-backed write methods can't run in unit tests (no pgx pool), so the
// fire point is exercised directly; every write method calls it after a
// successful write.
func TestFireConfigChanged(t *testing.T) {
	t.Parallel()

	type fired struct {
		key      string
		cfg      FlagConfig
		ctxAlive bool
	}

	ch := make(chan fired, 1)

	c := newEvalTestChecker()
	c.SetOnConfigChanged(func(ctx context.Context, key string, cfg FlagConfig) {
		ch <- fired{key: key, cfg: cfg, ctxAlive: ctx.Err() == nil}
	})

	want := FlagConfig{Phase: "testing", ShowOnRoadmap: true, RolloutPercentage: 40}

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // simulate the request context dying on response

	c.fireConfigChanged(ctx, "feature.test", want)

	select {
	case got := <-ch:
		if got.key != "feature.test" || !reflect.DeepEqual(got.cfg, want) {
			t.Errorf("hook payload = %v, %+v — want feature.test, %+v", got.key, got.cfg, want)
		}

		if !got.ctxAlive {
			t.Error("hook context was canceled — must be detached via context.WithoutCancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("config-change hook did not fire")
	}
}

// TestFireConfigChanged_NoHook verifies a nil hook is a no-op (no panic) —
// the checker must work before server.go wires the roadmap sync.
func TestFireConfigChanged_NoHook(t *testing.T) {
	t.Parallel()

	c := newEvalTestChecker()
	c.fireConfigChanged(t.Context(), "feature.test", FlagConfig{})
}

// TestHandlersSetOnConfigChanged_Delegates verifies the Handlers setter is a
// thin delegate to the checker's registry — writes through /control/extensions
// (which bypass Handlers) and writes through admin endpoints must hit the
// same hook.
func TestHandlersSetOnConfigChanged_Delegates(t *testing.T) {
	t.Parallel()

	c := newEvalTestChecker()
	h := NewHandlers(c)

	ch := make(chan string, 1)

	h.SetOnConfigChanged(func(_ context.Context, key string, _ FlagConfig) {
		ch <- key
	})

	c.fireConfigChanged(t.Context(), "feature.test", FlagConfig{})

	select {
	case key := <-ch:
		if key != "feature.test" {
			t.Errorf("hook key = %q, want feature.test", key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("delegated config-change hook did not fire")
	}
}

// TestFlagWriteHandlers_InvalidConfig verifies the unified-rollout config
// fields on FeatureFlagInput (phase, surfaces, show_on_roadmap) are validated
// by ValidateConfig before any write — invalid values must 400 with the
// ConfigError's machine-readable wire code (invalid_phase, invalid_surface,
// invalid_surface_state, invalid_audience, invalid_geo), never persist.
func TestFlagWriteHandlers_InvalidConfig(t *testing.T) {
	t.Parallel()

	h := NewHandlers(newEvalTestChecker())

	tests := []struct {
		name     string
		input    FeatureFlagInput
		wantCode string
	}{
		{"invalid phase", FeatureFlagInput{IsEnabled: true, Phase: "rolling"}, "invalid_phase"},
		{"unknown surface", FeatureFlagInput{IsEnabled: true, Surfaces: map[string]SurfaceRule{"web": {State: "visible"}}}, "invalid_surface"},
		{"invalid surface state", FeatureFlagInput{IsEnabled: true, Surfaces: map[string]SurfaceRule{"main": {State: "dimmed"}}}, "invalid_surface_state"},
		{"invalid audience entry", FeatureFlagInput{IsEnabled: true, Surfaces: map[string]SurfaceRule{"main": {State: "visible", Audience: []string{"beta"}}}}, "invalid_audience"},
		{"lowercase geo code", FeatureFlagInput{IsEnabled: true, Surfaces: map[string]SurfaceRule{"main": {State: "visible", GeoAllow: []string{"ng"}}}}, "invalid_geo"},
	}

	for _, tt := range tests {
		t.Run(tt.name+" (update)", func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			h.UpdateFeatureFlag(rec, adminFlagWriteReq(t, http.MethodPut, "feature.test", tt.input))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
			}

			if !strings.Contains(rec.Body.String(), `"`+tt.wantCode+`"`) {
				t.Errorf("body must carry %s code, got %s", tt.wantCode, rec.Body.String())
			}
		})
	}

	// Create path — same validation contract.
	rec := httptest.NewRecorder()
	h.CreateFeatureFlag(rec, adminFlagWriteReq(t, http.MethodPost, "", FeatureFlagInput{
		Key:   "feature.new",
		Phase: "rolling",
	}))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), `"invalid_phase"`) {
		t.Errorf("create body must carry invalid_phase code, got %s", rec.Body.String())
	}
}

// TestFlagConfigPatch verifies the patch object the admin write handlers hand
// to PatchFlag: scalar fields always ride the patch (PUT replace semantics),
// while phase/showOnRoadmap/surfaces appear only when present in the request
// body — so `config || patch` preserves values set via Control → Extensions.
func TestFlagConfigPatch(t *testing.T) {
	t.Parallel()

	cfg := FlagConfig{
		Name:              "Blog",
		Description:       "desc",
		Environment:       "production",
		RolloutPercentage: 100,
		Phase:             "shipped",
		ShowOnRoadmap:     true,
		Surfaces:          map[string]SurfaceRule{"main": {State: "hidden"}},
	}

	patchKeys := func(t *testing.T, body string) map[string]json.RawMessage {
		t.Helper()

		var m map[string]json.RawMessage
		if err := json.Unmarshal(flagConfigPatch(cfg, []byte(body)), &m); err != nil {
			t.Fatalf("patch unmarshal: %v", err)
		}

		return m
	}

	t.Run("absent rollout fields stay out of the patch", func(t *testing.T) {
		t.Parallel()

		m := patchKeys(t, `{"isEnabled":false,"name":"Blog"}`)
		if _, ok := m["phase"]; ok {
			t.Error("phase present in patch, want omitted")
		}

		if _, ok := m["showOnRoadmap"]; ok {
			t.Error("showOnRoadmap present in patch, want omitted")
		}

		if _, ok := m["surfaces"]; ok {
			t.Error("surfaces present in patch, want omitted")
		}

		if string(m["name"]) != `"Blog"` || string(m["rolloutPercentage"]) != `100` {
			t.Errorf("scalar fields missing from patch: %v", m)
		}
	})

	t.Run("present rollout fields ride the patch", func(t *testing.T) {
		t.Parallel()

		m := patchKeys(t, `{"phase":"shipped","showOnRoadmap":false,"surfaces":{"main":{"state":"hidden"}}}`)
		if string(m["phase"]) != `"shipped"` {
			t.Errorf("phase = %s, want shipped", m["phase"])
		}

		if string(m["showOnRoadmap"]) != `true` {
			t.Errorf("showOnRoadmap = %s, want true (cfg value)", m["showOnRoadmap"])
		}

		if _, ok := m["surfaces"]; !ok {
			t.Error("surfaces missing from patch")
		}
	})

	t.Run("explicit null surfaces clears via the patch", func(t *testing.T) {
		t.Parallel()

		nilCfg := FlagConfig{Name: "Blog"}

		var m map[string]json.RawMessage

		if err := json.Unmarshal(flagConfigPatch(nilCfg, []byte(`{"surfaces":null}`)), &m); err != nil {
			t.Fatalf("patch unmarshal: %v", err)
		}

		if string(m["surfaces"]) != `null` {
			t.Errorf("surfaces = %s, want explicit null", m["surfaces"])
		}
	})
}

// TestFlagWriteHandlers_ValidConfigReachesDB verifies a fully valid config
// passes validation (it can only fail at the DB layer with a nil-pool test
// checker — a 500 proves the request reached the write, not a 400).
func TestFlagWriteHandlers_ValidConfigReachesDB(t *testing.T) {
	t.Parallel()

	h := NewHandlers(newEvalTestChecker())

	rec := httptest.NewRecorder()
	h.UpdateFeatureFlag(rec, adminFlagWriteReq(t, http.MethodPut, "feature.test", FeatureFlagInput{
		IsEnabled:     true,
		Phase:         "testing",
		ShowOnRoadmap: true,
		Surfaces: map[string]SurfaceRule{
			"main": {State: "visible", Audience: []string{"group:beta"}, GeoDeny: []string{"NG"}},
		},
	}))

	if rec.Code == http.StatusBadRequest {
		t.Fatalf("valid config got 400: %s", rec.Body.String())
	}
}
