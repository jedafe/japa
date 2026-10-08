package features

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
