package jce

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// The JCE freshness and advisor cache keys must match the single-colon
// patterns documented in conventions/valkey-keys-catalog.md. A previous
// implementation wrote "jc:jce:freshness::{id}" / "jc:jce:advisor::{id}:..."
// which the cataloged invalidation sites (e.g. comparison
// DeleteJCEComparisonCaches) could never match.
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

func TestAdvisorCacheKeyMatchesCatalog(t *testing.T) {
	var contentID, profileID pgtype.UUID
	if err := contentID.Scan(uuid.New().String()); err != nil {
		t.Fatalf("contentID: %v", err)
	}

	if err := profileID.Scan(uuid.New().String()); err != nil {
		t.Fatalf("profileID: %v", err)
	}

	key := advisorCacheKey(contentID, profileID, "analyze|question|")

	if !strings.HasPrefix(key, "jc:jce:advisor:"+contentID.String()+":"+profileID.String()+":") {
		t.Fatalf("advisorCacheKey does not match catalog pattern jc:jce:advisor:{content}:{profile}:{hash}: %q", key)
	}

	if strings.Contains(key, "::") {
		t.Fatalf("advisorCacheKey contains double colon: %q", key)
	}

	// A nil profile must use the "none" segment so profile-scoped
	// invalidations (jc:jce:advisor:*:none:*) still match.
	noProfile := advisorCacheKey(contentID, pgtype.UUID{}, "analyze|question|")
	if !strings.Contains(noProfile, ":none:") {
		t.Fatalf("advisorCacheKey without profile missing :none: segment: %q", noProfile)
	}
}
