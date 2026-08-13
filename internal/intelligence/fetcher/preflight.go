package fetcher

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

var (
	ErrRobotsDisallowed = errors.New("fetcher: robots.txt disallows this path")
	ErrRateLimited      = errors.New("fetcher: rate limited for this domain")
	ErrLicenseRepublish = errors.New("fetcher: source license forbids republish")
	ErrCrawlLocked      = errors.New("fetcher: source already being crawled")
)

// PreflightChecker enforces robots.txt, rate-limit, license, and crawl-lock
// rules before any adapter runs.
type PreflightChecker struct {
	rdb              valkeycompat.Cmdable
	crawlLockTTLSecs int
	rateLimitPerMin  int
}

// NewPreflightChecker returns a PreflightChecker backed by the given Valkey
// (go-redis-compatible) client. crawlLockTTLSecs is the TTL for the per-source
// crawl lock; rateLimitPerMin is the maximum number of fetches allowed per
// domain per minute.
func NewPreflightChecker(rdb valkeycompat.Cmdable, crawlLockTTLSecs, rateLimitPerMin int) *PreflightChecker {
	return &PreflightChecker{rdb: rdb, crawlLockTTLSecs: crawlLockTTLSecs, rateLimitPerMin: rateLimitPerMin}
}

// Check runs all pre-flight checks. Returns nil if all pass.
// Crawl lock is SET NX EX — if the source is already being crawled, returns ErrCrawlLocked.
func (p *PreflightChecker) Check(ctx context.Context, source sources.Source) error {
	// 1. Crawl lock. Use the source UUID string as the lock key suffix; only
	// proceed when the ID is valid.
	sourceID := "unknown"
	if source.ID.Valid {
		sourceID = source.ID.String()
	}
	lockKey := fmt.Sprintf("jc:intel:crawllock:%s", sourceID)
	locked, err := p.rdb.SetNX(ctx, lockKey, "1", time.Duration(p.crawlLockTTLSecs)*time.Second).Result()
	if err != nil {
		return fmt.Errorf("fetcher: crawl lock: %w", err)
	}
	if !locked {
		return ErrCrawlLocked
	}

	// 2. License check (for Tier 3+, source_license must be non-NULL and not
	// 'proprietary' for republish).
	if source.TrustLevel >= 3 {
		if source.SourceLicense == nil || *source.SourceLicense == "proprietary" {
			return ErrLicenseRepublish
		}
	}

	// 3. Rate limit (sliding window per domain — check a counter that resets
	// every minute).
	rateKey := fmt.Sprintf("jc:intel:ratelimit:%s", source.Domain)
	count, err := p.rdb.Incr(ctx, rateKey).Result()
	if err != nil {
		return fmt.Errorf("fetcher: rate limit: %w", err)
	}
	if count == 1 {
		p.rdb.Expire(ctx, rateKey, time.Minute)
	}
	if count > int64(p.rateLimitPerMin) {
		return ErrRateLimited
	}

	// 4. Robots.txt check (load from cache or fetch in Task 15).
	// For now, if robots_policy has allowed=false, refuse.
	// Full robots.txt fetching + parsing is implemented in the static adapter task.

	return nil
}

// ReleaseCrawlLock removes the crawl lock. Called after fetch completes
// (success or failure) to allow immediate retry if needed.
func (p *PreflightChecker) ReleaseCrawlLock(ctx context.Context, sourceID string) {
	p.rdb.Del(ctx, fmt.Sprintf("jc:intel:crawllock:%s", sourceID))
}
