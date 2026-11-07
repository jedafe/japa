// Egress-proxy picking for the intelligence fetchers.
//
// Sites that wall the crawler's datacenter egress IP (Cloudflare,
// DataDome) can be routed through an operator-configured forward-proxy
// pool instead of the host's own address. The hook is deliberately
// narrow:
//
//   - Only domains explicitly mapped in intel.egress.proxy_domains
//     (JSON {domain: group}, suffix-matched) use a pool — every other
//     request egresses directly. A proxy is a trust/anonymity posture
//     decision per site, not a global default.
//   - Groups resolve through intel.egress.proxy_pools
//     (JSON {group: [proxy-url, ...]}). Proxy URLs are
//     http[s]://[user:pass@]host:port. Pool entries must be PUBLIC
//     addresses — the transport dial guard still vets the proxy dial
//     itself, so a private-literal proxy host fails closed (an internal
//     forward proxy is out of scope for egress diversity).
//   - Assignment is sticky per domain: jc:intel:egress_proxy:{domain}
//     (SET NX, TTL intel.egress.proxy_sticky_secs) pins one egress IP so
//     an anti-bot vendor does not see the same session rotate addresses.
//     Without Valkey the pick is still deterministic (host-keyed hash),
//     which is effectively sticky until the pool membership changes.
//
// SSRF boundary: once a proxy is engaged the upstream proxy resolves and
// dials the target, so the transport's private-IP dial guard can only
// see the proxy address. Two compensations hold: the proxy picker still
// refuses private-IP-LITERAL targets (a http://169.254.169.254/ entry is
// never proxied), and CheckRedirect keeps validating redirect targets.
// proxy_domains must therefore only list vetted external crawl targets —
// an operator mapping an internal hostname here would route internal
// traffic through the proxy.
//
// Ref: intel bot-wall hardening — egress diversity (§3 design, wayback
// escalation covers most walls; this is the datacenter-IP fallback).
package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"
)

// egressProxyKey is the Valkey key pattern for a domain's sticky proxy
// assignment — cataloged in valkey-keys-catalog (intel bot-wall section).
func egressProxyKey(domain string) string {
	return "jc:intel:egress_proxy:" + domain
}

// EgressProxyPicker resolves the per-request forward proxy for the
// transport. settings reads the pool/domain maps hot (system_settings);
// rdb holds the sticky assignment (nil = deterministic hash pick only).
type EgressProxyPicker struct {
	settings AdapterSettings
	rdb      valkeycompat.Cmdable
}

// NewEgressProxyPicker returns a picker, or nil when settings is absent —
// callers pass the result into SafeClientOpts.Proxy, where nil means
// "no proxy hook" (identical to today).
func NewEgressProxyPicker(settings AdapterSettings, rdb valkeycompat.Cmdable) *EgressProxyPicker {
	if settings == nil {
		return nil
	}

	return &EgressProxyPicker{settings: settings, rdb: rdb}
}

// ProxyFor implements the http.Transport.Proxy signature: nil = direct.
func (p *EgressProxyPicker) ProxyFor(req *http.Request) (*url.URL, error) {
	domain := strings.ToLower(req.URL.Hostname())
	if domain == "" {
		return nil, nil
	}

	// A private IP literal as the target must never be proxied — the
	// transport dial guard sees only the proxy address once a proxy is
	// engaged, so the literal check has to happen here. Hostname targets
	// are resolved upstream; proxy_domains is the vetting boundary.
	// isNumericIPPrivate covers the decimal/hex/octal IPv4 spellings
	// (2130706433, 0x7f.0.0.1) that net.ParseIP rejects but a proxy or
	// browser would happily dial.
	if ip := net.ParseIP(strings.Trim(domain, "[]")); ip != nil && isPrivateIP(ip) {
		return nil, errors.New("fetcher: refusing to proxy a private IP target")
	}

	if isNumericIPPrivate(domain) {
		return nil, errors.New("fetcher: refusing to proxy a private IP target")
	}

	group := p.groupFor(req.Context(), domain)
	if group == "" {
		return nil, nil
	}

	pool := p.poolFor(req.Context(), group)
	if len(pool) == 0 {
		return nil, nil
	}

	raw := p.stickyPick(req.Context(), domain, pool)

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, nil // bad pool entry — degrade to direct, not a fetch failure
	}

	return u, nil
}

// groupFor returns the proxy group mapped to domain by
// intel.egress.proxy_domains — exact match first, then suffix so
// "example.com" covers "www.example.com".
func (p *EgressProxyPicker) groupFor(ctx context.Context, domain string) string {
	var m map[string]string
	if err := json.Unmarshal([]byte(p.settings.GetStringOr(ctx, "intel.egress.proxy_domains", "{}")), &m); err != nil {
		return ""
	}

	// Keys normalize to lowercase — "Example.COM" in the setting must
	// still match the request's lowercased hostname.
	if g, ok := m[domain]; ok {
		return g
	}

	// Suffix match: the LONGEST matching key wins — map iteration order is
	// random, so a.b.walled.example matching both ".walled.example" and
	// ".example" must resolve to the more specific entry deterministically.
	best, bestLen := "", 0

	for k, g := range m {
		lk := strings.ToLower(k)
		if lk == domain {
			return g
		}

		if len(lk) > bestLen && strings.HasSuffix(domain, "."+lk) {
			best, bestLen = g, len(lk)
		}
	}

	return best
}

// poolFor returns the sanitized proxy-URL list for a group from
// intel.egress.proxy_pools.
func (p *EgressProxyPicker) poolFor(ctx context.Context, group string) []string {
	var m map[string][]string
	if err := json.Unmarshal([]byte(p.settings.GetStringOr(ctx, "intel.egress.proxy_pools", "{}")), &m); err != nil {
		return nil
	}

	var out []string

	for _, raw := range m[group] {
		if u, err := url.Parse(strings.TrimSpace(raw)); err == nil && u.Host != "" {
			out = append(out, strings.TrimSpace(raw))
		}
	}

	return out
}

// stickyPick returns the domain's pinned proxy: the Valkey assignment when
// it still names a pool member, else a deterministic hash pick claimed
// with SET NX so a concurrent crawl on the same host lands on the same
// egress IP.
func (p *EgressProxyPicker) stickyPick(ctx context.Context, domain string, pool []string) string {
	if p.rdb != nil {
		if cur, err := p.rdb.Get(ctx, egressProxyKey(domain)).Result(); err == nil {
			for _, candidate := range pool {
				if candidate == cur {
					return cur
				}
			}
		}
	}

	// Deterministic pick — without Valkey this is already stable for a
	// fixed pool; the SET NX only guards the first-pick race.
	h := fnv.New32a()
	_, _ = h.Write([]byte(domain))
	pick := pool[int(h.Sum32())%len(pool)]

	ttl := time.Duration(p.settings.GetIntOr(ctx, "intel.egress.proxy_sticky_secs", 86400)) * time.Second
	if p.rdb != nil && ttl > 0 {
		// NX so a racing crawl's claim wins; on contention re-read and
		// take whatever was claimed so both callers agree. A zero/negative
		// TTL means "no persistence" — the hash pick stays deterministic.
		if err := p.rdb.SetNX(ctx, egressProxyKey(domain), pick, ttl).Err(); err == nil {
			if cur, err := p.rdb.Get(ctx, egressProxyKey(domain)).Result(); err == nil {
				for _, candidate := range pool {
					if candidate == cur {
						return cur
					}
				}
			}
		}
	}

	return pick
}

// NewEgressClient returns a SafeClient whose transport routes mapped
// domains through the configured egress pools — the wiring shortcut for
// callers that want the hook without touching SafeClientOpts directly.
// Unconfigured pools produce the same client as NewSafeClient.
func NewEgressClient(timeout time.Duration, maxBody int64, settings AdapterSettings, rdb valkeycompat.Cmdable) *http.Client {
	return NewSafeClientOpts(timeout, maxBody, SafeClientOpts{Proxy: egressProxyFunc(settings, rdb)})
}

// egressProxyFunc adapts the picker to http.Transport.Proxy — nil picker
// or nil return means direct egress (the production default when
// intel.egress.proxy_domains is unset).
func egressProxyFunc(settings AdapterSettings, rdb valkeycompat.Cmdable) func(*http.Request) (*url.URL, error) {
	p := NewEgressProxyPicker(settings, rdb)
	if p == nil {
		return nil
	}

	return p.ProxyFor
}
