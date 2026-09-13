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
// Valkey under jc:intel:robots:{}:{domain} (1hr TTL per
// valkey-keys-catalog, tenant-scoped per ADR-112 Amendment 2).
type RobotsPolicy struct {
	Allowed         bool     `json:"allowed"`
	CrawlDelaySecs  int      `json:"crawl_delay_secs"`
	DisallowedPaths []string `json:"disallowed_paths"`
}

// RobotsChecker fetches and caches robots.txt policies per domain. It
// respects the crawl-delay directive and disallow rules. The cache is
// stored in Valkey under jc:intel:robots:{}:{domain} with a
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

	// Try cache first. A corrupt cache entry falls through to the fetch below.
	if cached, err := c.rdb.Get(ctx, cacheKey).Result(); err == nil {
		var policy RobotsPolicy
		if err := json.Unmarshal([]byte(cached), &policy); err == nil {
			return policy, nil
		}
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return RobotsPolicy{}, fmt.Errorf("robots: new request: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect-Intelligence/1.0 (+https://japaconnect.com)")

	resp, err := c.client.Do(req)
	if err != nil {
		// Try http fallback.
		robotsURL = fmt.Sprintf("http://%s/robots.txt", domain)

		req, err2 := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
		if err2 != nil {
			return RobotsPolicy{}, fmt.Errorf("robots: new request (http): %w", err2)
		}

		req.Header.Set("User-Agent", "JapaConnect-Intelligence/1.0 (+https://japaconnect.com)")

		resp, err = c.client.Do(req)
		if err != nil {
			return RobotsPolicy{}, fmt.Errorf("robots: fetch: %w", err)
		}
	}

	defer func() { _ = resp.Body.Close() }()

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

// robotsParser accumulates the robots.txt rules for a single document while
// tracking which User-agent block each rule belongs to. A dedicated
// "japaconnect"/"japaconnect-intelligence" block takes precedence over the
// wildcard "*" block.
type robotsParser struct {
	policy          RobotsPolicy
	currentUA       string
	inJapaBlock     bool
	inWildcardBlock bool
	disallowed      []string
}

// parseRobotsTXT parses a robots.txt body and extracts the rules applicable
// to our user agent. We look for User-agent: * (wildcard) rules. If a
// specific JapaConnect block exists, it takes precedence.
func parseRobotsTXT(body []byte) RobotsPolicy {
	p := &robotsParser{policy: RobotsPolicy{Allowed: true}}

	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		p.applyLine(line, strings.ToLower(line))
	}

	if len(p.disallowed) > 0 {
		p.policy.DisallowedPaths = p.disallowed
	}

	return p.policy
}

// applyLine dispatches a single robots.txt line: User-agent headers switch
// the active block, rule lines are collected when the block applies to us.
func (p *robotsParser) applyLine(line, lower string) {
	if strings.HasPrefix(lower, "user-agent:") {
		p.applyUserAgent(line)
		return
	}

	// Only collect rules from the JapaConnect block or the wildcard block.
	// JapaConnect rules take precedence — wildcard rules are only collected
	// while no JapaConnect rule has been seen.
	collect := p.inJapaBlock || (p.inWildcardBlock && len(p.disallowed) == 0)
	if !collect && p.currentUA == "" {
		return
	}

	p.applyRule(line, lower, collect)
}

// applyUserAgent starts a new User-agent block, flagging whether it is the
// dedicated JapaConnect block or the wildcard.
func (p *robotsParser) applyUserAgent(line string) {
	uaLower := strings.ToLower(strings.TrimSpace(line[len("User-agent:"):]))
	p.currentUA = uaLower

	switch uaLower {
	case "japaconnect", "japaconnect-intelligence":
		p.inJapaBlock = true
		p.inWildcardBlock = false
	case "*":
		p.inJapaBlock = false
		p.inWildcardBlock = true
	default:
		p.inJapaBlock = false
		p.inWildcardBlock = false
	}
}

// applyRule applies a Disallow/Allow/Crawl-delay line. collect indicates
// whether the current block applies to our user agent.
func (p *robotsParser) applyRule(line, lower string, collect bool) {
	switch {
	case strings.HasPrefix(lower, "disallow:"):
		path := strings.TrimSpace(line[len("Disallow:"):])
		if path == "" {
			// "Disallow:" with empty value means allow everything.
			p.policy.Allowed = true
			p.disallowed = nil

			return
		}

		if collect {
			p.disallowed = append(p.disallowed, path)
		}
	case strings.HasPrefix(lower, "allow:"):
		if path := strings.TrimSpace(line[len("Allow:"):]); path == "/" && collect {
			p.policy.Allowed = true
		}
	case strings.HasPrefix(lower, "crawl-delay:"):
		delayStr := strings.TrimSpace(line[len("Crawl-delay:"):])
		if delay, err := parseIntSafe(delayStr); err == nil && collect {
			p.policy.CrawlDelaySecs = delay
		}
	}
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
