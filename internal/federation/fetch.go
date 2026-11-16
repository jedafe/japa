package federation

// SSRF-guarded HTTP client for remote AP fetches (actor docs, key
// resolution, deliveries). Mirrors internal/media/remote_proxy.go's
// guard: resolve-and-dial-the-validated-IP closes the DNS-rebinding
// TOCTOU, redirects re-validate every hop, HTTPS only. Outbound fetches
// additionally throttle per-domain (jc:fed:fetch:{domain}) — a
// misbehaving remote must not turn our resolver into a hammer.

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	fetchMaxBody    = 1 << 20 // 1 MB response cap (§126)
	fetchThrottleMS = 2000    // jc:fed:fetch:{domain} — 1 req / 2 s
	fetchMaxWait    = 3 * time.Second
)

// fetchCIDRs covers non-public ranges the net.IP helpers miss — the same
// set media's proxy rejects (CGNAT, benchmarking, reserved, TEST-NET).
var fetchCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"100.64.0.0/10",
		"192.0.0.0/24",
		"198.18.0.0/15",
		"240.0.0.0/4",
		"192.0.2.0/24",
		"198.51.100.0/24",
		"203.0.113.0/24",
	}
	var out []*net.IPNet
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}()

// publicIP reports whether ip is globally routable. Loopback, private
// RFC1918, link-local, CGNAT, ULA, multicast, unspecified, and reserved
// space are all rejected — a remote instance must never aim our fetches
// at the backend network.
func publicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() ||
		ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return false
	}
	for _, n := range fetchCIDRs {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// fetchTransport resolves the host, rejects any non-public answer, and
// dials the validated IP directly — re-resolving at dial time would
// re-open the DNS answer window (rebinding TOCTOU). TLS ServerName and
// the Host header still come from the request URL.
var fetchTransport = &http.Transport{
	DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("federation fetch: resolving %s: %w", host, err)
		}
		var target net.IP
		for _, ip := range ips {
			if !publicIP(ip) {
				return nil, fmt.Errorf("federation fetch: %s resolves to non-public IP %s", host, ip)
			}
			if target == nil {
				target = ip
			}
		}
		if target == nil {
			return nil, fmt.Errorf("federation fetch: %s has no addresses", host)
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(target.String(), port))
	},
}

// fetchClient is the SSRF-guarded client. Redirects are re-validated per
// hop — a public entry URL may not 302 into the backend network.
var fetchClient = &http.Client{
	Timeout:   15 * time.Second,
	Transport: fetchTransport,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("federation fetch: too many redirects")
		}
		return checkFetchURL(req.URL)
	},
}

// checkFetchURL enforces the outbound allowlist: https only, public
// hostnames (literal IPs are validated at dial, but a private literal
// should fail before resolution is even attempted).
func checkFetchURL(u *url.URL) error {
	if u == nil || !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("federation fetch: https only")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !publicIP(ip) {
		return fmt.Errorf("federation fetch: %s is not a public address", ip)
	}
	return nil
}

// throttleDomain applies the per-domain fetch budget. Best-effort: it
// waits up to fetchMaxWait for the marker, then proceeds — politeness
// should never deadlock the inbox path, and correctness doesn't depend
// on the limiter holding.
func (s *Service) throttleDomain(ctx context.Context, domain string) {
	if s.vk == nil || domain == "" {
		return
	}
	key := "jc:fed:fetch:" + strings.ToLower(domain)
	deadline := time.Now().Add(fetchMaxWait)
	for {
		ok, err := s.vk.SetNX(ctx, key, "1", fetchThrottleMS*time.Millisecond).Result()
		if err != nil || ok {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// fetchAP GETs a remote AP document through the guarded client. The
// Accept header asks for the activity profile — peers content-negotiate
// actor/object docs on it.
func (s *Service) fetchAP(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("federation fetch: bad url: %w", err)
	}
	if err := checkFetchURL(u); err != nil {
		return nil, err
	}
	s.throttleDomain(ctx, u.Hostname())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", `application/activity+json, application/ld+json; profile="https://www.w3.org/ns/activitystreams", application/json`)
	req.Header.Set("User-Agent", "JapaConnect/"+s.cfg.APIBase)

	resp, err := fetchClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("federation fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("federation fetch: %s: status %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBody+1))
	if err != nil {
		return nil, fmt.Errorf("federation fetch: read: %w", err)
	}
	if len(body) > fetchMaxBody {
		return nil, fmt.Errorf("federation fetch: response exceeds %d bytes", fetchMaxBody)
	}
	return body, nil
}

// deliverAP POSTs a signed activity through the guarded client. The
// caller sets headers — this only enforces the URL checks + throttle.
func (s *Service) deliverAP(ctx context.Context, req *http.Request) (*http.Response, error) {
	u, err := url.Parse(req.URL.String())
	if err != nil {
		return nil, fmt.Errorf("federation deliver: bad url: %w", err)
	}
	if err := checkFetchURL(u); err != nil {
		return nil, err
	}
	s.throttleDomain(ctx, u.Hostname())
	return fetchClient.Do(req)
}

// tlsConfig keeps a modern floor on outbound federation connections.
func init() {
	fetchTransport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
}

var _ = valkey.Nil // referenced so the valkey import documents SetNX usage
