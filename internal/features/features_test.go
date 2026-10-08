package features

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
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

	c.cacheSet("feature.blog", true)

	val := c.cacheGet("feature.blog")
	if val == nil {
		t.Fatal("expected cached value, got nil")
	}

	if !*val {
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
// features.Gate(featureChecker, "key") literals and asserts each key exists in
// Registry. This keeps the registry from drifting out of sync with the routes
// it gates — a new Gate call without a Registry entry fails this test.
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
		if !ok || sel.Sel.Name != "Gate" {
			return true
		}
		if len(call.Args) < 2 {
			return true
		}
		if lit, ok := call.Args[1].(*ast.BasicLit); ok {
			gated = append(gated, strings.Trim(lit.Value, `"`))
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
