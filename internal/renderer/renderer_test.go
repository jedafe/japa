package renderer

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCacheKey(t *testing.T) {
	t.Parallel()

	// Use a valid relative path so New succeeds; the cacheKey logic is
	// independent of the script path content.
	r, err := New("tiptap-ssr.ts", nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	var tenantID pgtype.UUID
	if err := tenantID.Scan("11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatal(err)
	}

	var contentID pgtype.UUID
	if err := contentID.Scan("550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Fatal(err)
	}

	updatedAt := pgtype.Timestamptz{Time: time.Unix(1234567890, 0), Valid: true}

	key := r.cacheKey(tenantID, contentID, updatedAt)

	// Key should start with the jc:render:{tenant_id}: prefix
	if len(key) < 10 {
		t.Fatalf("cache key too short: %s", key)
	}

	if key[:11] != "jc:render:1" {
		t.Fatalf("cache key prefix wrong: %s", key)
	}

	// Same inputs must produce the same key (deterministic)
	key2 := r.cacheKey(tenantID, contentID, updatedAt)
	if key != key2 {
		t.Fatalf("cache key not deterministic: %s != %s", key, key2)
	}

	// Different timestamp must produce a different key
	updatedAt2 := pgtype.Timestamptz{Time: time.Unix(1234567891, 0), Valid: true}

	key3 := r.cacheKey(tenantID, contentID, updatedAt2)
	if key == key3 {
		t.Fatalf("different timestamps produced same key: %s", key)
	}

	// Different tenant must produce a different key (§4.2 tenant scoping)
	var tenantID2 pgtype.UUID
	if err := tenantID2.Scan("22222222-2222-2222-2222-222222222222"); err != nil {
		t.Fatal(err)
	}
	key4 := r.cacheKey(tenantID2, contentID, updatedAt)
	if key == key4 {
		t.Fatalf("different tenants produced same key: %s", key)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	// Valid relative .ts path inside the default allowlist root.
	r, err := New("tiptap-ssr.ts", nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if r == nil {
		t.Fatal("New returned nil")
	}

	if r.cache != nil {
		t.Fatal("cache should be nil")
	}

	if r.cacheTTL != 24*time.Hour {
		t.Fatalf("cacheTTL = %v, want 24h", r.cacheTTL)
	}
}

func TestNew_RejectsInvalidScriptPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{"absolute path", "/etc/passwd"},
		{"parent traversal", "../escape.ts"},
		{"nested parent traversal", "sub/../../escape.ts"},
		{"non-js extension", "tiptap-ssr.txt"},
		{"empty", ""},
		{"executable", "tiptap-ssr.sh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(tt.path, nil); err == nil {
				t.Errorf("New(%q) expected error, got nil", tt.path)
			}
		})
	}
}
