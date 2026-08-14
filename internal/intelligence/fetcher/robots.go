package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// RobotsPolicy is the parsed robots.txt policy for a domain, cached in
// Valkey under jc:intel:robots:{tenant_id}:{domain} (1hr TTL per
// valkey-keys-catalog, tenant-scoped per ADR-112 Amendment 2).
type RobotsPolicy struct {
	Allowed         bool     `json:"allowed"`
	CrawlDelaySecs  int      `json:"crawl_delay_secs"`
	DisallowedPaths []string `json:"disallowed_paths"`
}

// RobotsChecker fetches and caches robots.txt policies per domain. It
// respects the crawl-delay directive and disallow rules. The cache is
// stored in Valkey under jc:intel:robots:{tenant_id}:{domain} with a
// 1-hour TTL (BUG-19 fix, valkey-keys-catalog Section INTEL, tenant-scoped
// per ADR-112 Amendment 2).
type RobotsChecker struct {
	rdb    valkeycompat.Cmdable
	client *http.Client
}

// NewRobotsChecker returns a RobotsChecker backed by the given Valkey
// client. HTTP fetches use a 10s timeout (robots.txt is small).
func NewRobotsChecker(rdb valkeycompat.Cmdable) *RobotsChecker {
	return &RobotsChecker{
		rdb:    rdb,
		client: NewSafeClient(10*time.Second, 1*1024*1024), // robots.txt is tiny
	}
}

// Check fetches the robots.txt policy for the given URL's domain, either
// from the Valkey cache or by fetching /robots.txt. Returns the policy
// and nil on success. If robots.txt cannot be fetched (404, network
// error), the default policy (allow all, no crawl-delay) is returned.
// The cache key is tenant-scoped per ADR-112 Amendment 2.
func (c *RobotsChecker) Check(ctx context.Context, tenantID pgtype.UUID, targetURL string) (RobotsPolicy, error) {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return RobotsPolicy{}, fmt.Errorf("robots: parse url: %w", err)
	}
	domain := parsed.Hostname()
	if domain == "" {
		return RobotsPolicy{}, fmt.Errorf("robots: empty domain in url %q", targetURL)
	}

	cacheKey := fmt.Sprintf("jc:intel:robots:%s:%s", tenantID.String(), domain)

	// Try cache first.
	if cached, err := c.rdb.Get(ctx, cacheKey).Result(); err == nil {
		var policy RobotsPolicy
		if err := json.Unmarshal([]byte(cached), &policy); err == nil {
			return policy, nil
		}
		// Corrupt cache — fall through to fetch.
	}

	// Fetch robots.txt.
	policy, err := c.fetchRobots(ctx, domain)
	if err != nil {
		// On fetch failure, default to allow-all (conservative: don't block
		// crawling because robots.txt is unreachable — the source's own
		// robots_policy JSON field is the authoritative override).
		policy = RobotsPolicy{Allowed: true}
	}

	// Cache the policy (1hr TTL per valkey-keys-catalog).
	if encoded, err := json.Marshal(policy); err == nil {
		c.rdb.Set(ctx, cacheKey, string(encoded), time.Hour)
	}

	return policy, nil
}

// fetchRobots fetches and parses /robots.txt for the given domain.
func (c *RobotsChecker) fetchRobots(ctx context.Context, domain string) (RobotsPolicy, error) {
	scheme := "https"
	// Try https first; if it fails, try http.
	robotsURL := fmt.Sprintf("%s://%s/robots.txt", scheme, domain)

	req, err := http.NewRequestWithContext(ctx, "GET", robotsURL, nil)
	if err != nil {
		return RobotsPolicy{}, fmt.Errorf("robots: new request: %w", err)
	}
	req.Header.Set("User-Agent", "JapaConnect-Intelligence/1.0 (+https://japaconnect.com)")

	resp, err := c.client.Do(req)
	if err != nil {
		// Try http fallback.
		robotsURL = fmt.Sprintf("http://%s/robots.txt", domain)
		req, err2 := http.NewRequestWithContext(ctx, "GET", robotsURL, nil)
		if err2 != nil {
			return RobotsPolicy{}, fmt.Errorf("robots: new request (http): %w", err2)
		}
		req.Header.Set("User-Agent", "JapaConnect-Intelligence/1.0 (+https://japaconnect.com)")
		resp, err = c.client.Do(req)
		if err != nil {
			return RobotsPolicy{}, fmt.Errorf("robots: fetch: %w", err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// No robots.txt — allow all.
		return RobotsPolicy{Allowed: true}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return RobotsPolicy{}, fmt.Errorf("robots: http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return RobotsPolicy{}, fmt.Errorf("robots: read body: %w", err)
	}

	return parseRobotsTXT(body), nil
}

// parseRobotsTXT parses a robots.txt body and extracts the rules applicable
// to our user agent. We look for User-agent: * (wildcard) rules. If a
// specific JapaConnect block exists, it takes precedence.
func parseRobotsTXT(body []byte) RobotsPolicy {
	policy := RobotsPolicy{Allowed: true}
	lines := strings.Split(string(body), "\n")

	// Track which user-agent block we're in. We prioritize a specific
	// "JapaConnect" block, falling back to "*".
	currentUA := ""
	inJapaBlock := false
	inWildcardBlock := false
	var disallowed []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		lower := strings.ToLower(line)

		if strings.HasPrefix(lower, "user-agent:") {
			ua := strings.TrimSpace(line[len("User-agent:"):])
			uaLower := strings.ToLower(ua)
			currentUA = uaLower
			if uaLower == "japaconnect" || uaLower == "japaconnect-intelligence" {
				inJapaBlock = true
				inWildcardBlock = false
			} else if uaLower == "*" {
				inJapaBlock = false
				inWildcardBlock = true
			} else {
				inJapaBlock = false
				inWildcardBlock = false
			}
			continue
		}

		// Only collect rules from the JapaConnect block or the wildcard
		// block. JapaConnect rules take precedence (collected first,
		// wildcard rules are only added if no JapaConnect block exists).
		collect := inJapaBlock || (inWildcardBlock && !inJapaBlock && len(disallowed) == 0)
		if !collect && currentUA == "" {
			continue
		}

		if strings.HasPrefix(lower, "disallow:") {
			path := strings.TrimSpace(line[len("Disallow:"):])
			if path == "" {
				// "Disallow:" with empty value means allow everything.
				policy.Allowed = true
				disallowed = nil
				continue
			}
			if collect {
				disallowed = append(disallowed, path)
			}
		} else if strings.HasPrefix(lower, "allow:") {
			path := strings.TrimSpace(line[len("Allow:"):])
			if path == "/" && collect {
				policy.Allowed = true
			}
		} else if strings.HasPrefix(lower, "crawl-delay:") {
			delayStr := strings.TrimSpace(line[len("Crawl-delay:"):])
			if delay, err := parseIntSafe(delayStr); err == nil && collect {
				policy.CrawlDelaySecs = delay
			}
		}
	}

	if len(disallowed) > 0 {
		policy.DisallowedPaths = disallowed
	}

	return policy
}

// IsPathDisallowed checks whether the given URL path matches any of the
// disallowed patterns in the policy. robots.txt paths are prefix matches.
func (p RobotsPolicy) IsPathDisallowed(path string) bool {
	for _, disallowed := range p.DisallowedPaths {
		if strings.HasPrefix(path, disallowed) {
			return true
		}
	}
	return false
}

func parseIntSafe(s string) (int, error) {
	s = strings.TrimSpace(s)
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
