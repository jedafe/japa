package jce

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// The JCE freshness cache key must match the single-colon pattern documented
// in conventions/valkey-keys-catalog.md. A previous implementation wrote
// "jc:jce:freshness::{id}" which the cataloged invalidation sites (e.g.
// comparison DeleteJCEComparisonCaches) could never match.
func TestFreshnessCacheKeyMatchesCatalog(t *testing.T) {
	var contentID pgtype.UUID
	if err := contentID.Scan(uuid.New().String()); err != nil {
		t.Fatalf("contentID: %v", err)
	}

	key := freshnessCacheKey(contentID)

	want := "jc:jce:freshness:" + contentID.String()
	if key != want {
		t.Fatalf("freshnessCacheKey = %q, want %q", key, want)
	}

	if strings.Contains(key, "::") {
		t.Fatalf("freshnessCacheKey contains double colon: %q", key)
	}
}
