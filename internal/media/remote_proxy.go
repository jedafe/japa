package media

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Remote image proxy (GET /api/v1/media/proxy?url=...).
//
// Some third-party assets — notably external source/favicon logos served to
// the sources directory — are blocked by the remote host's
// Cross-Origin-Resource-Policy / ORB, so the browser cannot embed them and
// logs console noise. This endpoint fetches the image server-side, caches it
// in Valkey, and serves it same-origin with an image/* content type.
//
// Security: the request URL is attacker-controlled, so every dial is checked
// against public-IP space (SSRF guard) — including redirect hops, since the
// custom DialContext validates each new connection. Bodies are capped at
// media.proxy.max_bytes and only image/* responses are served; everything
// else falls through to the placeholder in the handler.
//
// Ref: conventions/api-routes-catalog.md (media), conventions/valkey-keys-catalog.md
// (jc:media:proxy:v2:{sha256(url)}).

// ProxyResult is a fetched remote image payload.
type ProxyResult struct {
	Body        []byte
	ContentType string
}

// proxyCacheEntry is the JSON payload cached under jc:media:proxy:v2:{sha256(url)}.
type proxyCacheEntry struct {
	ContentType string `json:"ct"`
	BodyB64     string `json:"body_b64"`
}

// SetValkey injects the Valkey client for remote-image proxy caching
// (jc:media:proxy:v2:{sha256(url)}, TTL from media.proxy.ttl_secs).
func (s *Service) SetValkey(vk valkeycompat.Cmdable) {
	s.vk = vk
}

// firstPartyProxyHosts are our own media/API hosts — derived from
// JC_IMGPROXY_BASE_URL, JC_API_PUBLIC_URL and JC_SITE_URL. They resolve to
// private/CGNAT addresses inside the backend network, which the SSRF guard
// below would reject, so the proxy could never serve our own imgproxy/serve
// URLs (they appeared wherever remoteImageUrl wrapped a first-party URL and
// rendered as the placeholder). The allowlist is a fixed set of configured
// hostnames — not a wildcard — so it cannot be used to smuggle arbitrary
// internal targets through the proxy.
var firstPartyProxyHosts = sync.OnceValue(func() map[string]bool {
	hosts := map[string]bool{}
	for _, env := range []string{"JC_IMGPROXY_BASE_URL", "JC_API_PUBLIC_URL", "JC_SITE_URL"} {
		u, err := url.Parse(strings.TrimSpace(os.Getenv(env)))
		if err != nil || u == nil || u.Hostname() == "" {
			continue
		}
		hosts[strings.ToLower(u.Hostname())] = true
	}
	return hosts
})

// proxyHTTPClient performs SSRF-safe outbound fetches: every connection is
// established through a DialContext that resolves the host and rejects any
// non-public IP, so DNS answers and redirect targets are validated at connect
// time on every hop.
var proxyHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("media proxy: resolving %s: %w", host, err)
			}

			// Dial the validated IP, not the hostname — re-resolving here
			// would re-open the DNS answer window (rebinding TOCTOU). TLS
			// ServerName and the Host header still come from the request
			// URL, so virtual hosting is unaffected.
			var target net.IP
			firstParty := firstPartyProxyHosts()[strings.ToLower(host)]
			for _, ip := range ips {
				if !firstParty && !isPublicIP(ip) {
					return nil, fmt.Errorf("media proxy: %s resolves to non-public IP %s", host, ip)
				}
				if target == nil {
					target = ip
				}
			}
			if target == nil {
				return nil, fmt.Errorf("media proxy: %s has no addresses", host)
			}

			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(target.String(), port))
		},
	},
}

// nonPublicCIDRs covers non-public ranges the net.IP helpers miss —
// IsPrivate only knows RFC1918/ULA and IsLinkLocal only the link-local
// blocks, so CGNAT, benchmarking, reserved, and protocol-assignment
// space would otherwise be dialed as "public".
var nonPublicCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"100.64.0.0/10",   // CGNAT (RFC 6598) — Tailscale/VPC internals
		"192.0.0.0/24",    // IETF protocol assignments (RFC 6890)
		"198.18.0.0/15",   // benchmark testing (RFC 2544)
		"240.0.0.0/4",     // reserved (includes 255.255.255.255 broadcast)
		"192.0.2.0/24",    // TEST-NET-1 (documentation)
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
	}
	var out []*net.IPNet
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}()

// isPublicIP reports whether ip is a globally routable address. Everything
// else — loopback, private RFC1918, link-local, CGNAT (100.64/10), ULA,
// multicast, unspecified, reserved — is rejected.
func isPublicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() ||
		ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return false
	}

	for _, n := range nonPublicCIDRs {
		if n.Contains(ip) {
			return false
		}
	}

	return true
}

// ProxyRemote fetches rawURL server-side (SSRF-guarded), caches the payload in
// Valkey, and returns the image bytes. Returns an error for non-image
// responses, oversize bodies, unreachable hosts, and disallowed targets.
func (s *Service) ProxyRemote(ctx context.Context, rawURL string) (ProxyResult, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return ProxyResult{}, errors.New("media proxy: invalid url")
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return ProxyResult{}, fmt.Errorf("media proxy: scheme %q not allowed", u.Scheme)
	}

	if u.User != nil || u.Host == "" {
		return ProxyResult{}, errors.New("media proxy: invalid host")
	}

	cacheKey := proxyCacheKey(rawURL)

	if s.vk != nil {
		if cached, err := s.vk.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var ent proxyCacheEntry
			if json.Unmarshal([]byte(cached), &ent) == nil && ent.ContentType != "" {
				if body, derr := base64.StdEncoding.DecodeString(ent.BodyB64); derr == nil {
					return ProxyResult{Body: body, ContentType: ent.ContentType}, nil
				}
			}
		}
	}

	maxBytes := int64(1 << 20) // 1 MiB
	ttlSecs := 86400

	if s.settings != nil {
		maxBytes = int64(s.settings.GetIntOr(ctx, "media.proxy.max_bytes", int(maxBytes)))
		ttlSecs = s.settings.GetIntOr(ctx, "media.proxy.ttl_secs", ttlSecs)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil) // #nosec G704 -- every dial is IP-validated by proxyHTTPClient below
	if err != nil {
		return ProxyResult{}, fmt.Errorf("media proxy: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect-MediaProxy/1.0")
	req.Header.Set("Accept", "image/*")

	resp, err := proxyHTTPClient.Do(req) // #nosec G704 -- DialContext rejects non-public IPs on every redirect hop
	if err != nil {
		return ProxyResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ProxyResult{}, fmt.Errorf("media proxy: upstream status %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(ct), "image/") {
		return ProxyResult{}, fmt.Errorf("media proxy: upstream content type %q is not an image", ct)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return ProxyResult{}, err
	}

	if int64(len(body)) > maxBytes {
		return ProxyResult{}, fmt.Errorf("media proxy: upstream body exceeds %d bytes", maxBytes)
	}

	// Re-encode to WebP before caching/serving — remote images get the same
	// compression pass as local uploads. optimizeImage returns the original
	// bytes when the body isn't a decodable raster (e.g. SVG), so it's safe
	// to call unconditionally on image/* responses.
	body, ct = optimizeImage(body, ct)

	if s.vk != nil && ttlSecs > 0 {
		ent, _ := json.Marshal(proxyCacheEntry{
			ContentType: ct,
			BodyB64:     base64.StdEncoding.EncodeToString(body),
		})
		s.vk.Set(ctx, cacheKey, string(ent), time.Duration(ttlSecs)*time.Second)
	}

	return ProxyResult{Body: body, ContentType: ct}, nil
}

// proxyCacheKey builds the Valkey key for a proxied URL
// (jc:media:proxy:v2:{sha256(url)} — conventions/valkey-keys-catalog.md).
func proxyCacheKey(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	// v2 suffix: entries written before the WebP re-encode was added hold
	// original bytes — they orphan on the new namespace and expire naturally.
	return "jc:media:proxy:v2:" + hex.EncodeToString(sum[:])
}
