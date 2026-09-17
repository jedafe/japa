package media

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
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
// (jc:media:proxy:{sha256(url)}).

// ProxyResult is a fetched remote image payload.
type ProxyResult struct {
	Body        []byte
	ContentType string
}

// proxyCacheEntry is the JSON payload cached under jc:media:proxy:{sha256(url)}.
type proxyCacheEntry struct {
	ContentType string `json:"ct"`
	BodyB64     string `json:"body_b64"`
}

// SetValkey injects the Valkey client for remote-image proxy caching
// (jc:media:proxy:{sha256(url)}, TTL from media.proxy.ttl_secs).
func (s *Service) SetValkey(vk valkeycompat.Cmdable) {
	s.vk = vk
}

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

			for _, ip := range ips {
				if !isPublicIP(ip) {
					return nil, fmt.Errorf("media proxy: %s resolves to non-public IP %s", host, ip)
				}
			}

			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(host, port))
		},
	},
}

// isPublicIP reports whether ip is a globally routable address. Everything
// else — loopback, private RFC1918, link-local, CGNAT (100.64/10), ULA,
// multicast, unspecified — is rejected.
func isPublicIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() &&
		!ip.IsLoopback() &&
		!ip.IsPrivate() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsMulticast() &&
		!ip.IsUnspecified()
}

// ProxyRemote fetches rawURL server-side (SSRF-guarded), caches the payload in
// Valkey, and returns the image bytes. Returns an error for non-image
// responses, oversize bodies, unreachable hosts, and disallowed targets.
func (s *Service) ProxyRemote(ctx context.Context, rawURL string) (ProxyResult, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return ProxyResult{}, fmt.Errorf("media proxy: invalid url")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return ProxyResult{}, fmt.Errorf("media proxy: scheme %q not allowed", u.Scheme)
	}
	if u.User != nil || u.Host == "" {
		return ProxyResult{}, fmt.Errorf("media proxy: invalid host")
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ProxyResult{}, fmt.Errorf("media proxy: %w", err)
	}
	req.Header.Set("User-Agent", "JapaConnect-MediaProxy/1.0")
	req.Header.Set("Accept", "image/*")

	resp, err := proxyHTTPClient.Do(req)
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
// (jc:media:proxy:{sha256(url)} — conventions/valkey-keys-catalog.md).
func proxyCacheKey(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return "jc:media:proxy:" + hex.EncodeToString(sum[:])
}
