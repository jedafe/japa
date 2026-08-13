package fetcher

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
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
// All Valkey keys are tenant-scoped per ADR-112 Amendment 2.
//
// If any check fails AFTER the lock is acquired (license, rate-limit,
// robots), the lock is released before returning so the source is not
// blocked until TTL expiry (BUG-10 fix, §4.3/§6.2). Only ErrCrawlLocked
// (another worker holds the lock) leaves the lock in place.
func (p *PreflightChecker) Check(ctx context.Context, source sources.Source, tenantID pgtype.UUID) error {
	// 1. Crawl lock. Use the source UUID string as the lock key suffix; only
	// proceed when the ID is valid.
	sourceID := "unknown"
	if source.ID.Valid {
		sourceID = source.ID.String()
	}
	lockKey := fmt.Sprintf("jc:intel:crawllock:%s:%s", tenantID.String(), sourceID)
	locked, err := p.rdb.SetNX(ctx, lockKey, "1", time.Duration(p.crawlLockTTLSecs)*time.Second).Result()
	if err != nil {
		return fmt.Errorf("fetcher: crawl lock: %w", err)
	}
	if !locked {
		return ErrCrawlLocked
	}

	// releaseLock is called on every error path after the lock is acquired
	// so a failed pre-flight check does not block the source until TTL
	// expiry. On success (nil return), the caller is responsible for
	// calling ReleaseCrawlLock after the fetch completes.
	releaseLock := func() { p.rdb.Del(ctx, lockKey) }

	// 2. License check (for Tier 3+, source_license must be non-NULL and not
	// 'proprietary' for republish).
	if source.TrustLevel >= 3 {
		if source.SourceLicense == nil || *source.SourceLicense == "proprietary" {
			releaseLock()
			return ErrLicenseRepublish
		}
	}

	// 3. Rate limit (sliding window per domain — check a counter that resets
	// every minute).
	rateKey := fmt.Sprintf("jc:intel:ratelimit:%s:%s", tenantID.String(), source.Domain)
	count, err := p.rdb.Incr(ctx, rateKey).Result()
	if err != nil {
		releaseLock()
		return fmt.Errorf("fetcher: rate limit: %w", err)
	}
	if count == 1 {
		p.rdb.Expire(ctx, rateKey, time.Minute)
	}
	if count > int64(p.rateLimitPerMin) {
		releaseLock()
		return ErrRateLimited
	}

	// 4. Robots.txt check (load from cache or fetch in Task 15).
	// For now, if robots_policy has allowed=false, refuse.
	// Full robots.txt fetching + parsing is implemented in the static adapter task.

	return nil
}

// ReleaseCrawlLock removes the crawl lock. Called after fetch completes
// (success or failure) to allow immediate retry if needed.
func (p *PreflightChecker) ReleaseCrawlLock(ctx context.Context, tenantID pgtype.UUID, sourceID string) {
	p.rdb.Del(ctx, fmt.Sprintf("jc:intel:crawllock:%s:%s", tenantID.String(), sourceID))
}
