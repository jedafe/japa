package features

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/geoip"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// startTestValkey spins up an in-memory Valkey (miniredis) and returns a
// go-redis-compatible client so the flag L2 cache can be tested without a
// real server. Mirrors the helper in internal/auth/bruteforce_valkey_test.go.
func startTestValkey(t *testing.T) valkeycompat.Cmdable {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}

	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}

	compat := valkeycompat.NewAdapter(vk)

	t.Cleanup(func() { vk.Close() })

	return compat
}

// newEvalTestChecker returns a Checker with an empty L1 cache and no DB/Valkey.
// EvalFlag/EvalAll only read rows through the cache in tests — any test key
// must be seeded via cacheSet/cacheAllSet or the registry default applies.
func newEvalTestChecker() *Checker {
	return &Checker{cache: &flagCache{
		entries: make(map[string]*flagCacheEntry),
		done:    make(chan struct{}),
	}}
}

// mustFlagConfig marshals a FlagConfig for seeding the L1 cache.
func mustFlagConfig(t *testing.T, cfg FlagConfig) []byte {
	t.Helper()

	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal FlagConfig: %v", err)
	}

	return b
}

func surfacesConfig(t *testing.T, rules map[string]SurfaceRule) []byte {
	t.Helper()

	return mustFlagConfig(t, FlagConfig{Surfaces: rules})
}

func TestSurfaceFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		header string
		want   string
	}{
		{"main", "main"},
		{"tools", "tools"},
		{"academy", "academy"},
		{"docs", "docs"},
		{"mobile", "mobile"},
		{"admin", "admin"},
		{"extension", "extension"},
		{"desktop", "desktop"},
		{"web", ""},   // unknown surface → no surface rule applied
		{"MAIN", ""},  // case-sensitive
		{"api", ""},   // bare API consumer
		{"main ", ""}, // whitespace is not trimmed — exact match only
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("X-JC-Surface", tt.header)
			}

			if got := SurfaceFrom(r); got != tt.want {
				t.Errorf("SurfaceFrom(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}

	t.Run("absent header", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if got := SurfaceFrom(r); got != "" {
			t.Errorf("SurfaceFrom(no header) = %q, want \"\"", got)
		}
	})
}

// evalCase is one row of the spec §5.1 resolution matrix.
type evalCase struct {
	name    string
	enabled bool
	config  []byte
	scope   CallerScope
	want    EvalResult
}

// runEvalCases seeds the L1 cache per case and asserts EvalFlag's result.
func runEvalCases(t *testing.T, tests []evalCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newEvalTestChecker()
			c.cacheSet("feature.test", tt.enabled, tt.config)

			if got := c.EvalFlag(t.Context(), "feature.test", tt.scope); got != tt.want {
				t.Errorf("EvalFlag() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestEvalFlag_SurfaceMatrix covers the surface-state leg of spec §5.1:
// enabled/disabled globals, visible/hidden/disabled surface states, and
// absent or unknown surfaces falling back to global behavior.
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §5.1.
func TestEvalFlag_SurfaceMatrix(t *testing.T) {
	t.Parallel()

	hiddenMain := surfacesConfig(t, map[string]SurfaceRule{
		"main": {State: "hidden"},
	})
	disabledMain := surfacesConfig(t, map[string]SurfaceRule{
		"main": {State: "disabled"},
	})
	hiddenTools := surfacesConfig(t, map[string]SurfaceRule{
		"tools": {State: "hidden"},
	})
	hiddenAudBeta := surfacesConfig(t, map[string]SurfaceRule{
		"main": {State: "hidden", Audience: []string{"group:beta"}},
	})

	runEvalCases(t, []evalCase{
		{
			name:    "disabled flag is off everywhere",
			enabled: false,
			scope:   CallerScope{Surface: "main", CountryCode: "US", GeoValid: true, SuperAdmin: true},
			want:    EvalResult{Enabled: false, Visible: false},
		},
		{
			name:    "enabled flag with no surface rules",
			enabled: true,
			scope:   CallerScope{Surface: "main"},
			want:    EvalResult{Enabled: true, Visible: true},
		},
		{
			name:    "enabled flag with empty config blob",
			enabled: true,
			config:  []byte(`{}`),
			scope:   CallerScope{Surface: "main"},
			want:    EvalResult{Enabled: true, Visible: true},
		},
		{
			name:    "hidden on main, caller on main",
			enabled: true,
			config:  hiddenMain,
			scope:   CallerScope{Surface: "main"},
			want:    EvalResult{Enabled: true, Visible: false},
		},
		{
			name:    "hidden on main, caller on tools",
			enabled: true,
			config:  hiddenMain,
			scope:   CallerScope{Surface: "tools"},
			want:    EvalResult{Enabled: true, Visible: true},
		},
		{
			name:    "hidden on main, no surface",
			enabled: true,
			config:  hiddenMain,
			scope:   CallerScope{},
			want:    EvalResult{Enabled: true, Visible: true},
		},
		{
			name:    "hidden on main, unknown surface treated as no surface",
			enabled: true,
			config:  hiddenMain,
			scope:   CallerScope{Surface: "web"},
			want:    EvalResult{Enabled: true, Visible: true},
		},
		{
			name:    "rule exists for other surface only",
			enabled: true,
			config:  hiddenTools,
			scope:   CallerScope{Surface: "main"},
			want:    EvalResult{Enabled: true, Visible: true},
		},
		{
			name:    "disabled on main",
			enabled: true,
			config:  disabledMain,
			scope:   CallerScope{Surface: "main"},
			want:    EvalResult{Enabled: false, Visible: false},
		},
		{
			name:    "disabled on main not bypassed by super_admin",
			enabled: true,
			config:  disabledMain,
			scope:   CallerScope{Surface: "main", SuperAdmin: true},
			want:    EvalResult{Enabled: false, Visible: false},
		},
		{
			name:    "hidden + audience, in audience",
			enabled: true,
			config:  hiddenAudBeta,
			scope:   CallerScope{Surface: "main", GroupSlugs: map[string]bool{"beta": true}},
			want:    EvalResult{Enabled: true, Visible: false},
		},
		{
			name:    "hidden + audience, out of audience is off",
			enabled: true,
			config:  hiddenAudBeta,
			scope:   CallerScope{Surface: "main"},
			want:    EvalResult{Enabled: false, Visible: false},
		},
	})
}

// TestEvalFlag_GeoMatrix covers the geo leg of spec §5.1: deny runs before
// allow, allow-lists fail closed on unknown country, deny-lists fail open,
// and super_admin does not bypass geo.
func TestEvalFlag_GeoMatrix(t *testing.T) {
	t.Parallel()

	allowUS := surfacesConfig(t, map[string]SurfaceRule{
		"main": {State: "visible", GeoAllow: []string{"US"}},
	})
	denyNG := surfacesConfig(t, map[string]SurfaceRule{
		"main": {State: "visible", GeoDeny: []string{"NG"}},
	})
	allowDenyUS := surfacesConfig(t, map[string]SurfaceRule{
		"main": {State: "visible", GeoAllow: []string{"US"}, GeoDeny: []string{"US"}},
	})

	runEvalCases(t, []evalCase{
		{
			name:    "geo_allow US, caller NG",
			enabled: true,
			config:  allowUS,
			scope:   CallerScope{Surface: "main", CountryCode: "NG", GeoValid: true},
			want:    EvalResult{Enabled: false, Visible: false},
		},
		{
			name:    "geo_allow US, caller US",
			enabled: true,
			config:  allowUS,
			scope:   CallerScope{Surface: "main", CountryCode: "US", GeoValid: true},
			want:    EvalResult{Enabled: true, Visible: true},
		},
		{
			name:    "geo_allow fails closed when country unknown",
			enabled: true,
			config:  allowUS,
			scope:   CallerScope{Surface: "main", GeoValid: false},
			want:    EvalResult{Enabled: false, Visible: false},
		},
		{
			name:    "geo_deny NG, caller NG",
			enabled: true,
			config:  denyNG,
			scope:   CallerScope{Surface: "main", CountryCode: "NG", GeoValid: true},
			want:    EvalResult{Enabled: false, Visible: false},
		},
		{
			name:    "geo_deny fails open when country unknown",
			enabled: true,
			config:  denyNG,
			scope:   CallerScope{Surface: "main", GeoValid: false},
			want:    EvalResult{Enabled: true, Visible: true},
		},
		{
			name:    "geo_deny NG, caller US",
			enabled: true,
			config:  denyNG,
			scope:   CallerScope{Surface: "main", CountryCode: "US", GeoValid: true},
			want:    EvalResult{Enabled: true, Visible: true},
		},
		{
			name:    "country in both allow and deny — deny wins",
			enabled: true,
			config:  allowDenyUS,
			scope:   CallerScope{Surface: "main", CountryCode: "US", GeoValid: true},
			want:    EvalResult{Enabled: false, Visible: false},
		},
		{
			name:    "super_admin does not bypass geo_allow",
			enabled: true,
			config:  allowUS,
			scope:   CallerScope{Surface: "main", CountryCode: "NG", GeoValid: true, SuperAdmin: true},
			want:    EvalResult{Enabled: false, Visible: false},
		},
	})
}

// TestEvalFlag_AudienceMatrix covers the audience leg of spec §5.1: group and
// perm matching, anonymous callers never matching, and the super_admin
// bypass.
func TestEvalFlag_AudienceMatrix(t *testing.T) {
	t.Parallel()

	audBeta := surfacesConfig(t, map[string]SurfaceRule{
		"main": {State: "visible", Audience: []string{"group:beta"}},
	})
	audPerm := surfacesConfig(t, map[string]SurfaceRule{
		"main": {State: "visible", Audience: []string{"perm:beta.access"}},
	})

	runEvalCases(t, []evalCase{
		{
			name:    "audience group match",
			enabled: true,
			config:  audBeta,
			scope:   CallerScope{Surface: "main", GroupSlugs: map[string]bool{"beta": true}},
			want:    EvalResult{Enabled: true, Visible: true},
		},
		{
			name:    "audience group miss — other group",
			enabled: true,
			config:  audBeta,
			scope:   CallerScope{Surface: "main", GroupSlugs: map[string]bool{"gamma": true}},
			want:    EvalResult{Enabled: false, Visible: false},
		},
		{
			name:    "audience group miss — anonymous caller",
			enabled: true,
			config:  audBeta,
			scope:   CallerScope{Surface: "main"},
			want:    EvalResult{Enabled: false, Visible: false},
		},
		{
			name:    "audience bypassed by super_admin",
			enabled: true,
			config:  audBeta,
			scope:   CallerScope{Surface: "main", SuperAdmin: true},
			want:    EvalResult{Enabled: true, Visible: true},
		},
		{
			name:    "audience perm match",
			enabled: true,
			config:  audPerm,
			scope:   CallerScope{Surface: "main", Permissions: map[string]bool{"beta.access": true}},
			want:    EvalResult{Enabled: true, Visible: true},
		},
		{
			name:    "audience perm miss",
			enabled: true,
			config:  audPerm,
			scope:   CallerScope{Surface: "main", Permissions: map[string]bool{"other.node": true}},
			want:    EvalResult{Enabled: false, Visible: false},
		},
	})
}

// TestEvalFlag_DefaultSurfaces verifies the registry's DefaultSurfaces apply
// at eval time for flags without a stored surface rule — the same merge
// Control → Extensions displays via buildExtensionEntry. A stored rule wins
// per-surface; registry defaults fill the gaps.
func TestEvalFlag_DefaultSurfaces(t *testing.T) {
	// Mutates the package-level Registry — must NOT be parallel.
	const key = "feature.testdefaults"
	Registry = append(Registry, Feature{
		Key:     key,
		Default: true,
		DefaultSurfaces: map[string]string{
			"admin": "hidden",
			"docs":  "disabled",
		},
	})

	t.Cleanup(func() { Registry = Registry[:len(Registry)-1] })

	c := newEvalTestChecker()
	ctx := t.Context()

	// No stored row — registry defaults drive the surface rules.
	if got := c.EvalFlag(ctx, key, CallerScope{Surface: "admin"}); got != (EvalResult{Enabled: true, Visible: false}) {
		t.Errorf("admin scope (no row) = %+v, want {true false} — default hidden", got)
	}

	if got := c.EvalFlag(ctx, key, CallerScope{Surface: "docs"}); got != (EvalResult{}) {
		t.Errorf("docs scope (no row) = %+v, want {false false} — default disabled", got)
	}

	if got := c.EvalFlag(ctx, key, CallerScope{Surface: "main"}); got != (EvalResult{Enabled: true, Visible: true}) {
		t.Errorf("main scope (no row) = %+v, want {true true} — no default rule", got)
	}

	// A stored row overriding one surface leaves the other defaults intact.
	c.cacheSet(key, true, surfacesConfig(t, map[string]SurfaceRule{
		"admin": {State: "visible"},
	}))

	if got := c.EvalFlag(ctx, key, CallerScope{Surface: "admin"}); got != (EvalResult{Enabled: true, Visible: true}) {
		t.Errorf("admin scope (stored override) = %+v, want {true true} — stored wins", got)
	}

	if got := c.EvalFlag(ctx, key, CallerScope{Surface: "docs"}); got != (EvalResult{}) {
		t.Errorf("docs scope (stored row) = %+v, want {false false} — default still applies", got)
	}

	// EvalAll applies the same merge for the registry-only seed path.
	if got := c.EvalAll(ctx, CallerScope{Surface: "docs"}); got[key] {
		t.Errorf("EvalAll[docs][%q] = true, want false — default disabled", key)
	}
}

// TestEvalFlag_RegistryDefaults verifies EvalFlag falls back to the registry
// default when the flag has no stored row — same fallback IsEnabled uses.
func TestEvalFlag_RegistryDefaults(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	c := newEvalTestChecker()

	if got := c.EvalFlag(ctx, "feature.blog", CallerScope{Surface: "main"}); got != (EvalResult{Enabled: true, Visible: true}) {
		t.Errorf("feature.blog (default on) = %+v, want {true true}", got)
	}

	if got := c.EvalFlag(ctx, "feature.wallet", CallerScope{Surface: "main"}); got != (EvalResult{Enabled: false, Visible: false}) {
		t.Errorf("feature.wallet (default off) = %+v, want {false false}", got)
	}
}

// TestIsEnabled_IgnoresScope verifies IsEnabled stays purely global — a flag
// enabled globally but disabled on "main" must still report true, since scope
// overlays live in EvalFlag only.
func TestIsEnabled_IgnoresScope(t *testing.T) {
	t.Parallel()

	c := newEvalTestChecker()
	c.cacheSet("feature.test", true, surfacesConfig(t, map[string]SurfaceRule{
		"main": {State: "disabled"},
	}))

	if !c.IsEnabled(t.Context(), "feature.test") {
		t.Error("IsEnabled = false, want true (global enabled is scope-independent)")
	}
}

// evalAllChecker builds a checker whose aggregate snapshot carries the four
// rows the EvalAll tests share.
func evalAllChecker(t *testing.T) *Checker {
	t.Helper()

	c := newEvalTestChecker()
	c.cacheAllSet([]FlagRow{
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

// TestEvalAll_Anonymous verifies the caller-scoped flags map for an anonymous
// caller: values are Enabled && Visible per flag, overlaid on registry
// defaults.
func TestEvalAll_Anonymous(t *testing.T) {
	t.Parallel()

	got := evalAllChecker(t).EvalAll(t.Context(), CallerScope{Surface: "main"})
	want := map[string]bool{
		"feature.hiddenmain": false, // hidden → excluded from map
		"feature.beta":       false, // anonymous is never in an audience
		"feature.off":        false,
		"feature.plain":      true,
		"feature.blog":       true,  // registry default, no row
		"feature.wallet":     false, // registry default off
	}

	for key, w := range want {
		if got[key] != w {
			t.Errorf("EvalAll[%q] = %v, want %v", key, got[key], w)
		}
	}
}

func TestEvalAll_AudienceMember(t *testing.T) {
	t.Parallel()

	got := evalAllChecker(t).EvalAll(t.Context(), CallerScope{
		Surface:    "main",
		GroupSlugs: map[string]bool{"beta": true},
	})

	if !got["feature.beta"] {
		t.Error("feature.beta = false, want true for beta member")
	}

	if got["feature.hiddenmain"] {
		t.Error("feature.hiddenmain = true, want false (still hidden)")
	}
}

func TestEvalAll_OtherSurface(t *testing.T) {
	t.Parallel()

	got := evalAllChecker(t).EvalAll(t.Context(), CallerScope{Surface: "tools"})

	if !got["feature.hiddenmain"] {
		t.Error("feature.hiddenmain = false on tools, want true (rule is main-only)")
	}

	if !got["feature.beta"] {
		t.Error("feature.beta = false on tools, want true (rule is main-only)")
	}
}

func TestEvalAll_NoSurface(t *testing.T) {
	t.Parallel()

	got := evalAllChecker(t).EvalAll(t.Context(), CallerScope{})

	if !got["feature.hiddenmain"] || !got["feature.beta"] {
		t.Error("no-surface scope must not apply surface rules")
	}
}

// stubResolver implements AudienceResolver for BuildScope tests.
type stubResolver struct {
	groups map[string]bool
	perms  map[string]bool
	super  bool
	err    error
	calls  int
}

func (s *stubResolver) ResolveScope(_ context.Context, _ pgtype.UUID) (map[string]bool, map[string]bool, bool, error) {
	s.calls++

	return s.groups, s.perms, s.super, s.err
}

// scopeTestReq builds a request optionally carrying an AuthenticatedUser and
// a GeoContext, mirroring what OptionalAuth + geoip.Middleware populate.
func scopeTestReq(withUser, withGeo bool) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := r.Context()

	if withUser {
		ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
			UserID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		})
	}

	if withGeo {
		ctx = context.WithValue(ctx, geoip.GeoContextKey, geoip.GeoContext{CountryCode: "NG", IsValid: true})
	}

	return r.WithContext(ctx)
}

func TestBuildScope_AuthedUser(t *testing.T) {
	t.Parallel()

	resolver := &stubResolver{
		groups: map[string]bool{"beta": true},
		perms:  map[string]bool{"beta.access": true},
	}
	c := newEvalTestChecker()
	c.SetAudienceResolver(resolver)

	sc := c.BuildScope(scopeTestReq(true, true), "main")

	if sc.Surface != "main" {
		t.Errorf("Surface = %q, want main", sc.Surface)
	}

	if sc.CountryCode != "NG" || !sc.GeoValid {
		t.Errorf("geo = %q/%v, want NG/true", sc.CountryCode, sc.GeoValid)
	}

	if !sc.GroupSlugs["beta"] || !sc.Permissions["beta.access"] {
		t.Errorf("audience = %v/%v, want beta group + perm", sc.GroupSlugs, sc.Permissions)
	}

	if resolver.calls != 1 {
		t.Errorf("resolver calls = %d, want 1", resolver.calls)
	}
}

func TestBuildScope_Anonymous(t *testing.T) {
	t.Parallel()

	resolver := &stubResolver{super: true}
	c := newEvalTestChecker()
	c.SetAudienceResolver(resolver)

	sc := c.BuildScope(scopeTestReq(false, true), "main")

	if sc.GroupSlugs != nil || sc.Permissions != nil || sc.SuperAdmin {
		t.Error("anonymous caller must have nil audience maps and SuperAdmin=false")
	}

	if resolver.calls != 0 {
		t.Errorf("resolver called %d times for anonymous caller, want 0", resolver.calls)
	}

	if sc.CountryCode != "NG" {
		t.Errorf("anonymous geo = %q, want NG (geo is IP-based, not auth-based)", sc.CountryCode)
	}
}

func TestBuildScope_ResolverError(t *testing.T) {
	t.Parallel()

	resolver := &stubResolver{err: errors.New("rbac unavailable")}
	c := newEvalTestChecker()
	c.SetAudienceResolver(resolver)

	sc := c.BuildScope(scopeTestReq(true, false), "main")

	if sc.GroupSlugs != nil || sc.Permissions != nil || sc.SuperAdmin {
		t.Error("resolver failure must leave caller anonymous (audience checks fail closed)")
	}
}

func TestBuildScope_NoResolver(t *testing.T) {
	t.Parallel()

	c := newEvalTestChecker()

	sc := c.BuildScope(scopeTestReq(true, true), "main")

	if sc.GroupSlugs != nil || sc.SuperAdmin {
		t.Error("nil resolver must not populate audience fields")
	}
}

func TestBuildScope_InvalidSurface(t *testing.T) {
	t.Parallel()

	c := newEvalTestChecker()

	if sc := c.BuildScope(scopeTestReq(false, false), "not-a-surface"); sc.Surface != "" {
		t.Errorf("Surface = %q, want \"\"", sc.Surface)
	}
}

// TestValkeyFlagValueRoundTrip verifies the JSON {enabled, config} L2 format
// and that the decoder accepts legacy bare values ("0"/"1"/"true"/"false")
// so in-flight entries written by older builds still resolve.
func TestValkeyFlagValueRoundTrip(t *testing.T) {
	t.Parallel()
	vk := startTestValkey(t)
	ctx := t.Context()

	c := newEvalTestChecker()
	c.vk = vk

	cfg := surfacesConfig(t, map[string]SurfaceRule{"main": {State: "hidden"}})
	c.valkeySet(ctx, "feature.rt", true, cfg)

	entry := c.valkeyGet(ctx, "feature.rt")
	if entry == nil {
		t.Fatal("valkeyGet after valkeySet returned nil")
	}

	if !entry.enabled {
		t.Error("round-trip enabled = false, want true")
	}

	if string(entry.config) != string(cfg) {
		t.Errorf("round-trip config = %s, want %s", entry.config, cfg)
	}

	legacy := []struct {
		key   string
		value string
		want  bool
	}{
		{"feature.legacy1", "1", true},
		{"feature.legacy0", "0", false},
		{"feature.legacytrue", "true", true},
		{"feature.legacyfalse", "false", false},
	}

	for _, tt := range legacy {
		if err := vk.Set(ctx, flagCacheKeyPrefix+tt.key, tt.value, 0).Err(); err != nil {
			t.Fatalf("seed %s: %v", tt.key, err)
		}

		entry := c.valkeyGet(ctx, tt.key)
		if entry == nil {
			t.Fatalf("valkeyGet(%q=%q) = nil, want enabled=%v", tt.key, tt.value, tt.want)
		}

		if entry.enabled != tt.want {
			t.Errorf("valkeyGet(%q=%q).enabled = %v, want %v", tt.key, tt.value, entry.enabled, tt.want)
		}

		if len(entry.config) != 0 {
			t.Errorf("legacy value %q should decode with empty config, got %s", tt.value, entry.config)
		}
	}

	if err := vk.Set(ctx, flagCacheKeyPrefix+"feature.garbage", "{not-json", 0).Err(); err != nil {
		t.Fatal(err)
	}

	if entry := c.valkeyGet(ctx, "feature.garbage"); entry != nil {
		t.Errorf("garbage value decoded as %+v, want nil (cache miss)", entry)
	}
}

// TestEvalFlag_ViaValkeyL2 verifies EvalFlag consumes the JSON L2 value —
// scoped rules stored in config apply on a cache hit with no DB.
func TestEvalFlag_ViaValkeyL2(t *testing.T) {
	t.Parallel()
	vk := startTestValkey(t)
	ctx := t.Context()

	raw := `{"enabled":true,"config":{"surfaces":{"main":{"state":"visible","geo_allow":["US"]}}}}`
	if err := vk.Set(ctx, flagCacheKeyPrefix+"feature.geo", raw, 0).Err(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c := newEvalTestChecker()
	c.vk = vk

	if got := c.EvalFlag(ctx, "feature.geo", CallerScope{Surface: "main", CountryCode: "NG", GeoValid: true}); got != (EvalResult{Enabled: false, Visible: false}) {
		t.Errorf("NG caller = %+v, want {false false}", got)
	}

	if got := c.EvalFlag(ctx, "feature.geo", CallerScope{Surface: "main", CountryCode: "US", GeoValid: true}); got != (EvalResult{Enabled: true, Visible: true}) {
		t.Errorf("US caller = %+v, want {true true}", got)
	}

	// Legacy bare value still resolves through EvalFlag (config = {}).
	if err := vk.Set(ctx, flagCacheKeyPrefix+"feature.legacy", "1", 0).Err(); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	if got := c.EvalFlag(ctx, "feature.legacy", CallerScope{Surface: "main"}); got != (EvalResult{Enabled: true, Visible: true}) {
		t.Errorf("legacy L2 hit = %+v, want {true true}", got)
	}
}

// TestAllFlagRows_AggregateCache verifies the jc:flags:_all L2 aggregate:
// an L2 hit returns every row without a DB query and warms L1.
func TestAllFlagRows_AggregateCache(t *testing.T) {
	t.Parallel()
	vk := startTestValkey(t)
	ctx := t.Context()

	rows := []FlagRow{
		{Key: "feature.a", Enabled: true, Config: []byte(`{"phase":"testing"}`)},
		{Key: "feature.b", Enabled: false},
	}

	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("marshal rows: %v", err)
	}

	if err := vk.Set(ctx, flagCacheKeyPrefix+flagAllCacheKey, string(raw), 0).Err(); err != nil {
		t.Fatalf("seed aggregate: %v", err)
	}

	c := newEvalTestChecker() // nil db — an aggregate miss would panic/fail
	c.vk = vk

	got := c.AllFlagRows(ctx)
	if len(got) != 2 || got[0].Key != "feature.a" || got[1].Key != "feature.b" {
		t.Fatalf("AllFlagRows = %+v, want 2 seeded rows", got)
	}

	if string(got[0].Config) != `{"phase":"testing"}` {
		t.Errorf("row config = %s, want the seeded blob", got[0].Config)
	}

	// Delete the L2 key — a second call must be served from L1.
	if err := vk.Del(ctx, flagCacheKeyPrefix+flagAllCacheKey).Err(); err != nil {
		t.Fatalf("del aggregate: %v", err)
	}

	got = c.AllFlagRows(ctx)
	if len(got) != 2 {
		t.Fatalf("AllFlagRows after L2 delete = %d rows, want 2 (L1 hit)", len(got))
	}
}

// TestClearCache_ClearsAggregate verifies ClearCache drops both the per-flag
// L1 entries and the aggregate rows snapshot.
func TestClearCache_ClearsAggregate(t *testing.T) {
	t.Parallel()

	c := newEvalTestChecker()
	c.cacheSet("feature.a", true, nil)
	c.cacheAllSet([]FlagRow{{Key: "feature.a", Enabled: true}})

	c.ClearCache()

	if len(c.cache.entries) != 0 {
		t.Errorf("entries not cleared: %d remain", len(c.cache.entries))
	}

	if c.cache.all != nil {
		t.Error("aggregate snapshot not cleared")
	}
}
