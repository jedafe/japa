package features

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	errpkg "github.com/jedafe/japa/internal/errors"
)

func TestDefaultFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"blog is on by default", "feature.blog", true},
		{"forum is on by default", "feature.forum", true},
		{"wallet is off by default", "feature.wallet", false},
		{"japabot is off by default", "feature.japabot", false},
		{"unknown feature defaults off", "feature.unknown", false},
		{"empty key defaults off", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := DefaultFor(tt.key)
			if got != tt.want {
				t.Errorf("DefaultFor(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestAllKeys(t *testing.T) {
	t.Parallel()

	keys := AllKeys()
	if len(keys) != len(Registry) {
		t.Errorf("AllKeys() returned %d keys, registry has %d", len(keys), len(Registry))
	}

	// V1 keys use the "feature.{name}" format (locked, Section 42).
	// V2 keys use the "{domain}.enabled" or "{domain}.{sub}" format per
	// conventions/feature-flag-naming.md and migration 000404.
	for _, k := range keys {
		if !strings.HasPrefix(k, "feature.") && !strings.Contains(k, ".") {
			t.Errorf("key %q does not follow V1 (feature.*) or V2 ({domain}.*) convention", k)
		}
	}
}

func TestChecker_ClearCache(t *testing.T) {
	t.Parallel()

	c := &Checker{
		cache: &flagCache{
			entries: map[string]*flagCacheEntry{
				"feature.blog": {enabled: true},
			},
		},
	}
	c.ClearCache()

	if len(c.cache.entries) != 0 {
		t.Errorf("cache should be empty, got %d entries", len(c.cache.entries))
	}
}

func TestChecker_CacheGetSet(t *testing.T) {
	t.Parallel()

	c := &Checker{
		cache: &flagCache{
			entries: make(map[string]*flagCacheEntry),
		},
	}

	c.cacheSet("feature.blog", true, nil)

	val := c.cacheGet("feature.blog")
	if val == nil {
		t.Fatal("expected cached value, got nil")
	}

	if !val.enabled {
		t.Error("expected true, got false")
	}

	val = c.cacheGet("feature.unknown")
	if val != nil {
		t.Error("expected nil for uncached key")
	}
}

func TestRegistryHasNoDuplicates(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)
	for _, f := range Registry {
		if seen[f.Key] {
			t.Errorf("duplicate feature key: %s", f.Key)
		}

		seen[f.Key] = true
	}
}

func TestRegistryHasChecklistEnabled(t *testing.T) {
	t.Parallel()

	if _, ok := RegistryLookup("checklist.enabled"); !ok {
		t.Error("checklist.enabled must be in the registry — it is used by features.Gate in the server")
	}
}

// TestRegistryCoversAllGatedKeys parses server.go for
// features.Gate(featureChecker, "key") and
// features.GateAny(featureChecker, "key", ...) literals and asserts each
// key exists in Registry. This keeps the registry from drifting out of
// sync with the routes it gates — a new Gate/GateAny call without a
// Registry entry fails this test.
func TestRegistryCoversAllGatedKeys(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "../server/server.go", nil, 0)
	if err != nil {
		t.Fatalf("parse server.go: %v", err)
	}

	var gated []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Gate" && sel.Sel.Name != "GateAny") {
			return true
		}
		if len(call.Args) < 2 {
			return true
		}
		// Gate takes one key (arg 1); GateAny is variadic (args 1..n).
		for _, arg := range call.Args[1:] {
			if lit, ok := arg.(*ast.BasicLit); ok {
				gated = append(gated, strings.Trim(lit.Value, `"`))
			}
		}
		return true
	})

	if len(gated) == 0 {
		t.Fatal("no features.Gate calls found in server.go — test may be broken")
	}

	for _, key := range gated {
		if _, ok := RegistryLookup(key); !ok {
			t.Errorf("gated flag %q missing from Registry", key)
		}
	}
}

// TestGateAny covers the composite-gate middleware: the request passes
// when ANY listed flag resolves enabled for the caller, and 404s with
// feature_disabled only when every key is off (or no keys are given).
// Flag state is seeded into the checker's L1 cache — an unseeded key
// falls through to its Registry default (all gated keys default off).
func TestGateAny(t *testing.T) {
	t.Parallel()

	newChecker := func(seeded map[string]bool) *Checker {
		c := &Checker{
			cache: &flagCache{entries: make(map[string]*flagCacheEntry)},
		}
		for k, v := range seeded {
			c.cacheSet(k, v, nil)
		}
		return c
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	serve := func(c *Checker, keys ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		rec := httptest.NewRecorder()
		GateAny(c, keys...)(next).ServeHTTP(rec, req)
		return rec
	}

	t.Run("second key enabled passes", func(t *testing.T) {
		t.Parallel()
		c := newChecker(map[string]bool{"mentorship.enabled": true})
		if rec := serve(c, "service_directory.enabled", "mentorship.enabled"); rec.Code != http.StatusNoContent {
			t.Fatalf("got %d, want 204", rec.Code)
		}
	})

	t.Run("first key enabled passes", func(t *testing.T) {
		t.Parallel()
		c := newChecker(map[string]bool{"service_directory.enabled": true})
		if rec := serve(c, "service_directory.enabled", "mentorship.enabled"); rec.Code != http.StatusNoContent {
			t.Fatalf("got %d, want 204", rec.Code)
		}
	})

	t.Run("all disabled returns 404 feature_disabled", func(t *testing.T) {
		t.Parallel()
		c := newChecker(nil)
		rec := serve(c, "service_directory.enabled", "mentorship.enabled")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "feature_disabled") {
			t.Fatalf("body missing feature_disabled: %s", rec.Body.String())
		}
	})

	t.Run("explicitly seeded-off key still 404s alongside an off default", func(t *testing.T) {
		t.Parallel()
		c := newChecker(map[string]bool{"service_directory.enabled": false})
		if rec := serve(c, "service_directory.enabled", "mentorship.enabled"); rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404", rec.Code)
		}
	})

	t.Run("empty keys list always 404s", func(t *testing.T) {
		t.Parallel()
		c := newChecker(map[string]bool{"service_directory.enabled": true})
		if rec := serve(c); rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404", rec.Code)
		}
	})
}

func TestChecker_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	c := &Checker{
		cache: &flagCache{
			entries: make(map[string]*flagCacheEntry),
			done:    make(chan struct{}),
		},
	}
	// Calling Close multiple times must not panic.
	c.Close()
	c.Close()
	c.Close()
}

func TestHumanizeFlagName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"feature.blog", "Feature Blog"},
		{"feature.dm", "Feature Dm"},
		{"feature.ai_content_gen", "Feature Ai Content Gen"},
		{"story.enabled", "Story Enabled"},
		{"forum.ama.enabled", "Forum Ama Enabled"},
		{"checklist.enabled", "Checklist Enabled"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := humanizeFlagName(tt.input)
			if got != tt.want {
				t.Errorf("humanizeFlagName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidPhase(t *testing.T) {
	t.Parallel()

	for _, p := range Phases {
		if !ValidPhase(p) {
			t.Errorf("ValidPhase(%q) = false, want true (listed in Phases)", p)
		}
	}

	for _, p := range []string{"", "beta", "SHIPPED", "in-progress", "released"} {
		if ValidPhase(p) {
			t.Errorf("ValidPhase(%q) = true, want false", p)
		}
	}
}

func TestValidSurface(t *testing.T) {
	t.Parallel()

	for _, s := range Surfaces {
		if !ValidSurface(s) {
			t.Errorf("ValidSurface(%q) = false, want true (listed in Surfaces)", s)
		}
	}

	for _, s := range []string{"", "web", "MAIN", "api", "ios"} {
		if ValidSurface(s) {
			t.Errorf("ValidSurface(%q) = true, want false", s)
		}
	}
}

// TestValidateConfig covers the FlagConfig validation rules from the
// unified-feature-rollout spec §3.1: phase vocabulary, known surface keys,
// per-surface state vocabulary, audience prefix format, and uppercase ISO
// alpha-2 geo codes.
func TestValidateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     FlagConfig
		wantErr string // expected invalid_* code substring; "" means valid
	}{
		{
			name:    "empty config is valid",
			cfg:     FlagConfig{},
			wantErr: "",
		},
		{
			name: "full valid config",
			cfg: FlagConfig{
				Name:              "Blog",
				Phase:             "testing",
				ShowOnRoadmap:     true,
				RolloutPercentage: 25,
				Surfaces: map[string]SurfaceRule{
					"main": {
						State:    "visible",
						Audience: []string{"group:beta_testers", "perm:beta.access"},
						GeoAllow: []string{"US", "CA"},
					},
					"tools":   {State: "hidden"},
					"academy": {State: "disabled", GeoDeny: []string{"NG"}},
				},
			},
			wantErr: "",
		},
		{
			name:    "every phase value accepted",
			cfg:     FlagConfig{Phase: "archived"},
			wantErr: "",
		},
		{
			name:    "invalid phase",
			cfg:     FlagConfig{Phase: "beta"},
			wantErr: "invalid_phase",
		},
		{
			name: "unknown surface key",
			cfg: FlagConfig{
				Phase:    "planned",
				Surfaces: map[string]SurfaceRule{"web": {State: "visible"}},
			},
			wantErr: "invalid_surface",
		},
		{
			name: "unknown surface among valid keys",
			cfg: FlagConfig{
				Surfaces: map[string]SurfaceRule{
					"main": {State: "visible"},
					"tv":   {State: "hidden"},
				},
			},
			wantErr: "invalid_surface",
		},
		{
			name: "invalid surface state",
			cfg: FlagConfig{
				Surfaces: map[string]SurfaceRule{"main": {State: "gone"}},
			},
			wantErr: "invalid_surface_state",
		},
		{
			name: "empty surface state rejected",
			cfg: FlagConfig{
				Surfaces: map[string]SurfaceRule{"main": {Audience: []string{"group:x"}}},
			},
			wantErr: "invalid_surface_state",
		},
		{
			name: "audience entry without prefix",
			cfg: FlagConfig{
				Surfaces: map[string]SurfaceRule{
					"main": {State: "visible", Audience: []string{"beta_testers"}},
				},
			},
			wantErr: "invalid_audience",
		},
		{
			name: "audience entry with wrong prefix",
			cfg: FlagConfig{
				Surfaces: map[string]SurfaceRule{
					"main": {State: "visible", Audience: []string{"user:123"}},
				},
			},
			wantErr: "invalid_audience",
		},
		{
			name: "lowercase geo_allow code rejected",
			cfg: FlagConfig{
				Surfaces: map[string]SurfaceRule{
					"main": {State: "visible", GeoAllow: []string{"us"}},
				},
			},
			wantErr: "invalid_geo",
		},
		{
			name: "lowercase geo_deny code rejected",
			cfg: FlagConfig{
				Surfaces: map[string]SurfaceRule{
					"main": {State: "visible", GeoDeny: []string{"ng"}},
				},
			},
			wantErr: "invalid_geo",
		},
		{
			name: "three-letter geo code rejected",
			cfg: FlagConfig{
				Surfaces: map[string]SurfaceRule{
					"main": {State: "visible", GeoAllow: []string{"USA"}},
				},
			},
			wantErr: "invalid_geo",
		},
		{
			name: "numeric geo code rejected",
			cfg: FlagConfig{
				Surfaces: map[string]SurfaceRule{
					"main": {State: "visible", GeoDeny: []string{"12"}},
				},
			},
			wantErr: "invalid_geo",
		},
		{
			name: "same code in geo_allow and geo_deny is valid (deny wins at eval)",
			cfg: FlagConfig{
				Surfaces: map[string]SurfaceRule{
					"main": {State: "visible", GeoAllow: []string{"US"}, GeoDeny: []string{"US"}},
				},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateConfig(tt.cfg)

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateConfig() = %v, want nil", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("ValidateConfig() = nil, want %q error", tt.wantErr)
			}

			if !errors.Is(err, errpkg.ErrInvalid) {
				t.Errorf("ValidateConfig() error %v does not wrap ErrInvalid", err)
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ValidateConfig() error %q missing code %q", err.Error(), tt.wantErr)
			}

			var ce *ConfigError
			if !errors.As(err, &ce) {
				t.Fatalf("ValidateConfig() error %T does not extract *ConfigError", err)
			}

			if ce.Code != tt.wantErr {
				t.Errorf("ConfigError.Code = %q, want %q", ce.Code, tt.wantErr)
			}
		})
	}
}
