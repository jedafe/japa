package renderer

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCacheKey(t *testing.T) {
	r := New("/path/to/script.ts", nil)

	var contentID pgtype.UUID
	if err := contentID.Scan("550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Fatal(err)
	}

	updatedAt := pgtype.Timestamptz{Time: time.Unix(1234567890, 0), Valid: true}

	key := r.cacheKey(contentID, updatedAt)

	// Key should start with the jc:render: prefix
	if len(key) < 10 {
		t.Fatalf("cache key too short: %s", key)
	}
	if key[:11] != "jc:render:5" {
		t.Fatalf("cache key prefix wrong: %s", key)
	}

	// Same inputs must produce the same key (deterministic)
	key2 := r.cacheKey(contentID, updatedAt)
	if key != key2 {
		t.Fatalf("cache key not deterministic: %s != %s", key, key2)
	}

	// Different timestamp must produce a different key
	updatedAt2 := pgtype.Timestamptz{Time: time.Unix(1234567891, 0), Valid: true}
	key3 := r.cacheKey(contentID, updatedAt2)
	if key == key3 {
		t.Fatalf("different timestamps produced same key: %s", key)
	}
}

func TestNew(t *testing.T) {
	r := New("/path/to/script.ts", nil)
	if r == nil {
		t.Fatal("New returned nil")
	}
	if r.scriptPath != "/path/to/script.ts" {
		t.Fatalf("scriptPath = %s, want /path/to/script.ts", r.scriptPath)
	}
	if r.cache != nil {
		t.Fatal("cache should be nil")
	}
	if r.cacheTTL != 24*time.Hour {
		t.Fatalf("cacheTTL = %v, want 24h", r.cacheTTL)
	}
}
