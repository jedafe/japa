package fetcher

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// startTestValkey spins up an in-memory Valkey (miniredis) and returns a
// go-redis-compatible client so the PreflightChecker can be tested without
// a real server. Follows the pattern established in internal/auth.
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
	t.Cleanup(func() { vk.Close() })

	return valkeycompat.NewAdapter(vk)
}

func newTestChecker(t *testing.T, crawlLockTTLSecs, rateLimitPerMin int) *PreflightChecker {
	t.Helper()
	return NewPreflightChecker(startTestValkey(t), crawlLockTTLSecs, rateLimitPerMin)
}

func validUUID() pgtype.UUID {
	// Deterministic UUID for tests; Valid=true so the crawl-lock key is stable.
	return pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Valid: true}
}

func TestPreflightCheck_CrawlLock(t *testing.T) {
	p := newTestChecker(t, 3600, 10)

	src := sources.Source{ID: validUUID(), Domain: "example.com", TrustLevel: 1}
	if err := p.Check(context.Background(), src); err != nil {
		t.Fatalf("first check: %v", err)
	}
	// Second check on same source should be locked.
	if err := p.Check(context.Background(), src); err != ErrCrawlLocked {
		t.Errorf("second check: got %v, want ErrCrawlLocked", err)
	}
}

func TestPreflightCheck_ReleaseCrawlLock(t *testing.T) {
	p := newTestChecker(t, 3600, 10)

	src := sources.Source{ID: validUUID(), Domain: "example.com", TrustLevel: 1}
	if err := p.Check(context.Background(), src); err != nil {
		t.Fatalf("first check: %v", err)
	}
	// Releasing the lock should allow a second check to pass.
	p.ReleaseCrawlLock(context.Background(), src.ID.String())
	if err := p.Check(context.Background(), src); err != nil {
		t.Errorf("check after release: got %v, want nil", err)
	}
}

func TestPreflightCheck_LicenseRepublish(t *testing.T) {
	p := newTestChecker(t, 3600, 10)

	proprietary := "proprietary"
	src := sources.Source{ID: validUUID(), Domain: "blog.example.com", TrustLevel: 3, SourceLicense: &proprietary}
	if err := p.Check(context.Background(), src); err != ErrLicenseRepublish {
		t.Errorf("got %v, want ErrLicenseRepublish", err)
	}
}

func TestPreflightCheck_LicenseNilTier3(t *testing.T) {
	p := newTestChecker(t, 3600, 10)

	// Tier 3 with no license at all should be refused.
	src := sources.Source{ID: validUUID(), Domain: "blog.example.com", TrustLevel: 3}
	if err := p.Check(context.Background(), src); err != ErrLicenseRepublish {
		t.Errorf("got %v, want ErrLicenseRepublish", err)
	}
}

func TestPreflightCheck_LicenseAllowedTier1(t *testing.T) {
	p := newTestChecker(t, 3600, 10)

	// Tier 1 (official) is not subject to the license check even with a
	// proprietary license.
	proprietary := "proprietary"
	src := sources.Source{ID: validUUID(), Domain: "gov.example.com", TrustLevel: 1, SourceLicense: &proprietary}
	if err := p.Check(context.Background(), src); err != nil {
		t.Errorf("tier 1 with proprietary license: got %v, want nil", err)
	}
}

func TestPreflightCheck_RateLimited(t *testing.T) {
	p := newTestChecker(t, 3600, 2)

	src := sources.Source{ID: validUUID(), Domain: "ratelimit.example.com", TrustLevel: 1}
	// First two checks pass (counter 1 and 2); the third exceeds the limit.
	if err := p.Check(context.Background(), src); err != nil {
		t.Fatalf("check 1: %v", err)
	}
	// Release the crawl lock between checks so only the rate limiter is
	// exercised.
	p.ReleaseCrawlLock(context.Background(), src.ID.String())
	if err := p.Check(context.Background(), src); err != nil {
		t.Fatalf("check 2: %v", err)
	}
	p.ReleaseCrawlLock(context.Background(), src.ID.String())
	if err := p.Check(context.Background(), src); err != ErrRateLimited {
		t.Errorf("check 3: got %v, want ErrRateLimited", err)
	}
}

func TestNormalizeForHash(t *testing.T) {
	// HTML content is whitespace-collapsed before hashing.
	html := []byte("<html>\n  <body>\tHello   World\n</body></html>")
	h1 := NormalizeForHash(html, "text/html; charset=utf-8")
	h2 := NormalizeForHash([]byte("<html> <body> Hello World </body></html>"), "text/html")
	if h1 != h2 {
		t.Errorf("whitespace-only differences should hash equal: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Error("expected non-empty hash")
	}

	// Non-HTML content is hashed raw, so different bytes differ.
	raw1 := NormalizeForHash([]byte("payload"), "application/json")
	raw2 := NormalizeForHash([]byte("payload2"), "application/json")
	if raw1 == raw2 {
		t.Error("distinct JSON bodies should hash differently")
	}
}
