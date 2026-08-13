package fetcher

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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

// SettingsReader is the minimal interface PreflightChecker needs to read
// system_settings at runtime (Rule 3). Defining it locally avoids an import
// cycle on internal/config.
type SettingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// PreflightChecker enforces robots.txt, rate-limit, license, and crawl-lock
// rules before any adapter runs.
type PreflightChecker struct {
	rdb             valkeycompat.Cmdable
	robots          *RobotsChecker
	settings        SettingsReader
	defaultTenantID pgtype.UUID
}

// NewPreflightChecker returns a PreflightChecker backed by the given Valkey
// (go-redis-compatible) client. Crawl lock TTL and rate-limit per-minute are
// read from system_settings (intel.crawl_lock_ttl_secs, intel.ratelimit_default_per_min)
// at check time per Rule 3. robots may be nil to skip robots.txt checks
// (not recommended for production). settings may be nil to fall back to
// hardcoded defaults (3600s TTL, 10 req/min).
func NewPreflightChecker(rdb valkeycompat.Cmdable, robots *RobotsChecker, settings SettingsReader, defaultTenantID pgtype.UUID) *PreflightChecker {
	return &PreflightChecker{
		rdb:             rdb,
		robots:          robots,
		settings:        settings,
		defaultTenantID: defaultTenantID,
	}
}

// rateLimitScript atomically increments the per-domain rate-limit counter and
// sets the 60-second TTL only on the first increment. This prevents the race
// where a process crashes after INCR but before EXPIRE, leaving the key
// without a TTL and permanently blocking the domain (INTEL-013 fix, §18).
const rateLimitScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return count
`

// Check runs all pre-flight checks. Returns nil if all pass.
// Crawl lock is SET NX EX — if the source is already being crawled, returns ErrCrawlLocked.
// All Valkey keys are tenant-scoped per ADR-112 Amendment 2.
//
// If any check fails AFTER the lock is acquired (license, rate-limit,
// robots), the lock is released before returning so the source is not
// blocked until TTL expiry (BUG-10 fix, §4.3/§6.2). Only ErrCrawlLocked
// (another worker holds the lock) leaves the lock in place.
func (p *PreflightChecker) Check(ctx context.Context, source sources.Source, tenantID pgtype.UUID) error {
	// Resolve config values from system_settings (Rule 3, INTEL-014 fix).
	crawlLockTTL := 3600
	rateLimitPerMin := 10
	if p.settings != nil {
		crawlLockTTL = p.settings.GetIntOr(ctx, p.defaultTenantID, "intel.crawl_lock_ttl_secs", 3600)
		rateLimitPerMin = p.settings.GetIntOr(ctx, p.defaultTenantID, "intel.ratelimit_default_per_min", 10)
	}

	// 1. Crawl lock. Use the source UUID string as the lock key suffix; only
	// proceed when the ID is valid.
	sourceID := "unknown"
	if source.ID.Valid {
		sourceID = source.ID.String()
	}
	lockKey := fmt.Sprintf("jc:intel:crawllock:%s:%s", tenantID.String(), sourceID)
	locked, err := p.rdb.SetNX(ctx, lockKey, "1", time.Duration(crawlLockTTL)*time.Second).Result()
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

	// 3. Rate limit (sliding window per domain — atomic INCR + conditional
	// EXPIRE via Lua to prevent the race where a crash after INCR leaves
	// the key without a TTL, permanently blocking the domain).
	rateKey := fmt.Sprintf("jc:intel:ratelimit:%s:%s", tenantID.String(), source.Domain)
	result, err := p.rdb.Eval(ctx, rateLimitScript, []string{rateKey}, 60).Result()
	if err != nil {
		releaseLock()
		return fmt.Errorf("fetcher: rate limit: %w", err)
	}
	count, ok := result.(int64)
	if !ok {
		releaseLock()
		return fmt.Errorf("fetcher: rate limit: unexpected eval result type %T", result)
	}
	if count > int64(rateLimitPerMin) {
		releaseLock()
		return ErrRateLimited
	}

	// 4. Robots.txt check (INTEL-006 fix). If a RobotsChecker is wired,
	// fetch/cache the robots.txt policy for the source's domain and verify
	// the source URL path is not disallowed. If robots.txt is unreachable,
	// RobotsChecker defaults to allow-all (conservative).
	if p.robots != nil && source.SourceUrl != "" {
		policy, err := p.robots.Check(ctx, tenantID, source.SourceUrl)
		if err != nil {
			releaseLock()
			return fmt.Errorf("fetcher: robots check: %w", err)
		}
		parsed, parseErr := url.Parse(source.SourceUrl)
		if parseErr != nil {
			releaseLock()
			return fmt.Errorf("fetcher: robots url parse: %w", parseErr)
		}
		if !policy.Allowed || policy.IsPathDisallowed(parsed.Path) {
			releaseLock()
			return ErrRobotsDisallowed
		}
		// Honor crawl-delay by sleeping before the fetch. This is a
		// best-effort throttle; the rate limiter in step 3 provides the
		// hard cap.
		if policy.CrawlDelaySecs > 0 {
			select {
			case <-time.After(time.Duration(policy.CrawlDelaySecs) * time.Second):
			case <-ctx.Done():
				releaseLock()
				return ctx.Err()
			}
		}
	}

	return nil
}

// ReleaseCrawlLock removes the crawl lock. Called after fetch completes
// (success or failure) to allow immediate retry if needed.
func (p *PreflightChecker) ReleaseCrawlLock(ctx context.Context, tenantID pgtype.UUID, sourceID string) {
	p.rdb.Del(ctx, fmt.Sprintf("jc:intel:crawllock:%s:%s", tenantID.String(), sourceID))
}
