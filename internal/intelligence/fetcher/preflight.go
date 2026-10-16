package fetcher

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// Exported var values.
var (
	// ErrRobotsDisallowed is a sentinel error: fetcher: robots.txt disallows this path.
	ErrRobotsDisallowed = errors.New("fetcher: robots.txt disallows this path")
	ErrRateLimited      = errors.New("fetcher: rate limited for this domain")
	ErrLicenseRepublish = errors.New("fetcher: source license forbids republish")
	ErrCrawlLocked      = errors.New("fetcher: source already being crawled")
)

// SettingsReader is the minimal interface PreflightChecker needs to read
// system_settings at runtime (Rule 3). Defining it locally avoids an import
// cycle on internal/config.
type SettingsReader interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// PreflightChecker enforces robots.txt, rate-limit, license, and crawl-lock
// rules before any adapter runs.
type PreflightChecker struct {
	rdb      valkeycompat.Cmdable
	robots   *RobotsChecker
	settings SettingsReader
}

// NewPreflightChecker returns a PreflightChecker backed by the given Valkey
// (go-redis-compatible) client. Crawl lock TTL and rate-limit per-minute are
// read from system_settings (intel.crawl_lock_ttl_secs, intel.ratelimit_default_per_min)
// at check time per Rule 3. robots may be nil to skip robots.txt checks
// (not recommended for production). settings may be nil to fall back to
// hardcoded defaults (3600s TTL, 10 req/min).
func NewPreflightChecker(rdb valkeycompat.Cmdable, robots *RobotsChecker, settings SettingsReader) *PreflightChecker {
	return &PreflightChecker{
		rdb:      rdb,
		robots:   robots,
		settings: settings,
	}
}

// rateLimitScript checks the per-domain window before admitting a request:
// when the counter is already at the limit it returns -1 WITHOUT
// incrementing, so rejected attempts (retries of rate-limited tasks) do not
// themselves consume the window and keep the domain saturated. Only an
// admitted request INCRs. The 60-second TTL is set on the first admitted
// increment — this prevents the race where a process crashes after INCR but
// before EXPIRE, leaving the key without a TTL and permanently blocking the
// domain (INTEL-013 fix, §18). ARGV[1] is the TTL seconds, ARGV[2] the
// per-minute limit.
const rateLimitScript = `
local count = tonumber(redis.call('GET', KEYS[1]) or '0')
if count >= tonumber(ARGV[2]) then
  return -1
end
count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return count
`

// Check runs all pre-flight checks. Returns nil if all pass.
// Crawl lock is SET NX EX — if the source is already being crawled, returns ErrCrawlLocked.
// All Valkey keys are platform-scoped per ADR-112 Amendment 2.
// // If any check fails AFTER the lock is acquired (license, rate-limit,
// robots), the lock is released before returning so the source is not
// blocked until TTL expiry (BUG-10 fix, §4.3/§6.2). Only ErrCrawlLocked
// (another worker holds the lock) leaves the lock in place.
func (p *PreflightChecker) Check(ctx context.Context, source sources.Source) error {
	// 1. Crawl lock. Use the source UUID string as the lock key suffix; only
	// proceed when the ID is valid.
	sourceID := "unknown"
	if source.ID.Valid {
		sourceID = source.ID.String()
	}

	lockKey := "jc:intel:crawllock::" + sourceID

	return p.check(ctx, lockKey, source.Domain, source.SourceUrl, source)
}

// CheckChannel is the channel-scoped counterpart of Check (intel-channels
// Phase 1): the crawl lock keys on the channel ID
// (jc:intel:channel_lock:{channel_id}) so two channels of the same source
// can crawl concurrently, the rate-limit domain is derived from the
// channel's entry_url host (a channel may point at a different host than
// source.domain), and robots.txt is evaluated against entry_url. The
// license check still reads the parent source's trust_level/source_license —
// republish policy is a property of the source, not the channel.
func (p *PreflightChecker) CheckChannel(ctx context.Context, src sources.Source, ch sources.Channel) error {
	channelID := "unknown"
	if ch.ID.Valid {
		channelID = ch.ID.String()
	}

	lockKey := "jc:intel:channel_lock:" + channelID

	domain := src.Domain
	if u, err := url.Parse(ch.EntryUrl); err == nil && u.Host != "" {
		domain = u.Host
	}

	// Search channels never fetch entry_url — the adapter calls the SERP
	// provider API — so robots against the informational entry URL is
	// meaningless and can strand the channel (a provider whose robots.txt
	// disallows everything would fail every poll). Lock/rate/license still
	// apply; robots is skipped.
	checkURL := ch.EntryUrl
	if sources.CrawlMethod(ch.CrawlMethod) == sources.CrawlMethodSearch {
		checkURL = ""
	}

	return p.check(ctx, lockKey, domain, checkURL, src)
}

// CheckChannelDerived is CheckChannel for a manifest-derived child fetch
// (Phase-2 sitemap/wayback fan-out): the crawl lock scopes to
// channel+child URL — jc:intel:channel_lock:{channel_id}:{url_hash} — so
// the bounded set of children fanned out from one manifest do not starve
// each other on the shared channel lock (only the first would fetch; the
// rest would answer ErrCrawlLocked and be lost). Rate limit and robots
// still key on the child URL's host.
func (p *PreflightChecker) CheckChannelDerived(ctx context.Context, src sources.Source, ch sources.Channel, childURL string) error {
	channelID := "unknown"
	if ch.ID.Valid {
		channelID = ch.ID.String()
	}

	domain := src.Domain
	if u, err := url.Parse(childURL); err == nil && u.Host != "" {
		domain = u.Host
	}

	return p.check(ctx, derivedChannelLockKey(channelID, childURL), domain, childURL, src)
}

// derivedChannelLockKey is the derived-fetch lock key shared by
// CheckChannelDerived and ReleaseChannelCrawlLockScoped.
func derivedChannelLockKey(channelID, childURL string) string {
	sum := sha256.Sum256([]byte(childURL))
	return fmt.Sprintf("jc:intel:channel_lock:%s:%x", channelID, sum[:8])
}

// ReleaseChannelCrawlLockScoped releases a derived-fetch crawl lock —
// the CheckChannelDerived counterpart of ReleaseChannelCrawlLock.
func (p *PreflightChecker) ReleaseChannelCrawlLockScoped(ctx context.Context, channelID, childURL string) {
	p.rdb.Del(ctx, derivedChannelLockKey(channelID, childURL))
}

// check runs the shared pre-flight gate: crawl lock (already keyed by the
// caller), license, per-domain rate limit, and robots.txt against checkURL.
func (p *PreflightChecker) check(ctx context.Context, lockKey, domain, checkURL string, source sources.Source) error {
	// Resolve config values from system_settings (Rule 3, INTEL-014 fix).
	crawlLockTTL := 3600
	rateLimitPerMin := 10

	if p.settings != nil {
		crawlLockTTL = p.settings.GetIntOr(ctx, "intel.crawl_lock_ttl_secs", 3600)
		rateLimitPerMin = p.settings.GetIntOr(ctx, "intel.ratelimit_default_per_min", 10)
	}

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

	// 3. Rate limit (sliding window per domain).
	if err := p.checkRateLimit(ctx, domain, rateLimitPerMin, releaseLock); err != nil {
		return err
	}

	// 4. Robots.txt check (INTEL-006 fix).
	return p.checkRobots(ctx, checkURL, releaseLock)
}

// checkRateLimit enforces the per-domain sliding-window rate limit via the
// atomic INCR + conditional EXPIRE Lua script (prevents the race where a
// crash after INCR leaves the key without a TTL, permanently blocking the
// domain). The lock is released on any failure.
func (p *PreflightChecker) checkRateLimit(ctx context.Context, domain string, rateLimitPerMin int, releaseLock func()) error {
	rateKey := "jc:intel:ratelimit::" + domain

	result, err := p.rdb.Eval(ctx, rateLimitScript, []string{rateKey}, 60, rateLimitPerMin).Result()
	if err != nil {
		releaseLock()
		return fmt.Errorf("fetcher: rate limit: %w", err)
	}

	count, ok := result.(int64)
	if !ok {
		releaseLock()
		return fmt.Errorf("fetcher: rate limit: unexpected eval result type %T", result)
	}

	// -1 = window already full: reject WITHOUT having incremented, so a
	// burst of rejected tasks cannot extend its own saturation.
	if count == -1 {
		releaseLock()
		return ErrRateLimited
	}

	return nil
}

// checkRobots verifies checkURL against the domain's robots.txt policy
// when a RobotsChecker is wired. If robots.txt is unreachable, RobotsChecker
// defaults to allow-all (conservative). The lock is released on any failure.
func (p *PreflightChecker) checkRobots(ctx context.Context, checkURL string, releaseLock func()) error {
	if p.robots == nil || checkURL == "" {
		return nil
	}

	policy, err := p.robots.Check(ctx, checkURL)
	if err != nil {
		releaseLock()
		return fmt.Errorf("fetcher: robots check: %w", err)
	}

	parsed, parseErr := url.Parse(checkURL)
	if parseErr != nil {
		releaseLock()
		return fmt.Errorf("fetcher: robots url parse: %w", parseErr)
	}

	if !policy.Allowed || policy.IsPathDisallowed(parsed.Path) {
		releaseLock()
		return ErrRobotsDisallowed
	}
	// Honor crawl-delay by sleeping before the fetch. This is a best-effort
	// throttle; the rate limiter provides the hard cap.
	if policy.CrawlDelaySecs <= 0 {
		return nil
	}

	select {
	case <-time.After(time.Duration(policy.CrawlDelaySecs) * time.Second):
		return nil
	case <-ctx.Done():
		releaseLock()
		return ctx.Err()
	}
}

// ReleaseCrawlLock removes the crawl lock. Called after fetch completes
// (success or failure) to allow immediate retry if needed.
func (p *PreflightChecker) ReleaseCrawlLock(ctx context.Context, sourceID string) {
	p.rdb.Del(ctx, "jc:intel:crawllock::"+sourceID)
}

// ReleaseChannelCrawlLock removes a channel-scoped crawl lock acquired by
// CheckChannel (jc:intel:channel_lock:{channel_id}).
func (p *PreflightChecker) ReleaseChannelCrawlLock(ctx context.Context, channelID string) {
	p.rdb.Del(ctx, "jc:intel:channel_lock:"+channelID)
}
