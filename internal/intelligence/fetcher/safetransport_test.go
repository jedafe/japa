package fetcher

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// stubConn is a minimal net.Conn for dial-function tests — the guarded
// dialer is exercised for its accept/reject decision, never for I/O.
type stubConn struct{ net.Conn }

func (stubConn) Close() error { return nil }

// TestGuardedDialContext_BlocksPrivateLiterals unit-tests the dial guard
// directly: every outbound connection attempt with a private/reserved IP
// literal host must be rejected before the wrapped dialer runs (the base
// dialer panics if it is ever called).
func TestGuardedDialContext_BlocksPrivateLiterals(t *testing.T) {
	t.Parallel()

	base := func(_ context.Context, _, _ string) (net.Conn, error) {
		panic("base dialer must not run for a private host")
	}

	dial := guardedDialContext(base)

	blocked := []string{
		"169.254.169.254:80", // cloud metadata / link-local
		"127.0.0.1:80",       // loopback
		"127.10.20.30:443",   // loopback (whole /8)
		"10.0.0.5:80",        // RFC1918 class A
		"172.16.1.1:80",      // RFC1918 class B
		"192.168.1.1:80",     // RFC1918 class C
		"100.64.7.7:80",      // CGNAT
		"0.0.0.0:80",         // current network
		"[::1]:80",           // IPv6 loopback
		"[fc00::1]:443",      // IPv6 ULA
		"[fe80::1]:80",       // IPv6 link-local
		"224.0.0.251:5353",   // IPv4 multicast
		"192.0.2.10:80",      // TEST-NET-1
		"255.255.255.255:80", // broadcast
	}

	for _, addr := range blocked {
		t.Run(addr, func(t *testing.T) {
			t.Parallel()

			conn, err := dial(t.Context(), "tcp", addr)
			if err == nil {
				_ = conn.Close()

				t.Fatalf("dial(%s) succeeded, want SSRF rejection", addr)
			}

			if !strings.Contains(err.Error(), "SSRF") {
				t.Fatalf("dial(%s) error = %v, want SSRF rejection", addr, err)
			}
		})
	}
}

// TestGuardedDialContext_PublicLiteralDials verifies a public IP literal
// passes the guard and reaches the wrapped dialer untouched.
func TestGuardedDialContext_PublicLiteralDials(t *testing.T) {
	t.Parallel()

	var gotAddr string

	base := func(_ context.Context, _, addr string) (net.Conn, error) {
		gotAddr = addr
		return stubConn{}, nil
	}

	dial := guardedDialContext(base)

	conn, err := dial(t.Context(), "tcp", "8.8.8.8:443")
	if err != nil {
		t.Fatalf("dial(8.8.8.8:443) = %v, want success", err)
	}

	_ = conn.Close()

	if gotAddr != "8.8.8.8:443" {
		t.Errorf("base received %q, want 8.8.8.8:443", gotAddr)
	}
}

// TestGuardedDialContext_DNSAnswers verifies the hostname path: every DNS
// answer is checked (one private answer rejects the whole dial), failures
// and empty answer sets fail closed, and a clean resolution dials a
// VALIDATED address — never re-resolved by the underlying dialer (DNS
// rebinding TOCTOU). ipLookup is stubbed; no real DNS runs.
func TestGuardedDialContext_DNSAnswers(t *testing.T) {
	// Not parallel — stubs the package-level ipLookup var.
	oldLookup := ipLookup

	t.Cleanup(func() { ipLookup = oldLookup })

	const host = "internal.example.com"

	cases := []struct {
		name    string
		lookup  func() ([]net.IPAddr, error)
		wantErr string // substring; "" means the base dialer must run
	}{
		{"single private answer rejects", func() ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("10.1.2.3")}}, nil
		}, "private IP"},
		{"private answer hidden behind public rejects", func() ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("169.254.169.254")}}, nil
		}, "private IP"},
		{"ipv6 ULA answer rejects", func() ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("fd00::7")}}, nil
		}, "private IP"},
		{"lookup error fails closed", func() ([]net.IPAddr, error) {
			return nil, errors.New("no such host")
		}, "lookup failed"},
		{"empty answer set fails closed", func() ([]net.IPAddr, error) {
			return nil, nil
		}, "no addresses"},
		{"public answers dial validated ip", func() ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ipLookup = func(_ context.Context, _ string) ([]net.IPAddr, error) {
				return tc.lookup()
			}

			var gotAddr string

			dial := guardedDialContext(func(_ context.Context, _, addr string) (net.Conn, error) {
				gotAddr = addr
				return stubConn{}, nil
			})

			conn, err := dial(t.Context(), "tcp", host+":443")
			if tc.wantErr != "" {
				if err == nil {
					_ = conn.Close()

					t.Fatalf("dial(%s) succeeded, want error containing %q", host, tc.wantErr)
				}

				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("dial(%s) error = %v, want substring %q", host, err, tc.wantErr)
				}

				if gotAddr != "" {
					t.Fatalf("base dialer ran with %q despite rejection", gotAddr)
				}

				return
			}

			if err != nil {
				t.Fatalf("dial(%s) = %v, want success", host, err)
			}

			_ = conn.Close()
			// The dial must target the VALIDATED address — the underlying
			// dialer is not allowed to re-resolve the hostname.
			if gotAddr != "93.184.216.34:443" {
				t.Errorf("base received %q, want validated 93.184.216.34:443", gotAddr)
			}
		})
	}
}

// TestNewSafeClient_BlocksPrivateLiteralFetch verifies the assembled client:
// a request to a private-IP literal host fails before connecting — the
// first hop is guarded, not just redirects.
func TestNewSafeClient_BlocksPrivateLiteralFetch(t *testing.T) {
	t.Parallel()

	client := NewSafeClient(5*time.Second, 0)
	for _, u := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://127.0.0.1:8080/admin",
		"http://10.0.0.1/internal",
	} {
		resp, err := client.Get(u)
		if err == nil {
			_ = resp.Body.Close()

			t.Errorf("GET %s succeeded, want SSRF rejection", u)

			continue
		}

		if !strings.Contains(err.Error(), "SSRF") {
			t.Errorf("GET %s error = %v, want SSRF rejection", u, err)
		}
	}
}

// TestNewSafeClientOpts_AllowPrivateReachesLoopback covers the documented
// test opt-out: with AllowPrivate the client reaches an httptest server on
// 127.0.0.1 — the escape hatch the worker/fetcher tests need.
func TestNewSafeClientOpts_AllowPrivateReachesLoopback(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	client := NewSafeClientOpts(5*time.Second, 0, SafeClientOpts{AllowPrivate: true})

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("AllowPrivate GET %s = %v, want success", srv.URL, err)
	}

	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestIsPrivateIP covers the range table behind the guards — IPv4-mapped
// IPv6 must be caught by the IPv4 CIDRs.
func TestIsPrivateIP(t *testing.T) {
	t.Parallel()

	private := []string{
		"127.0.0.1", "10.9.8.7", "172.31.0.1", "192.168.0.254",
		"169.254.169.254", "100.64.0.1", "0.0.0.0", "192.0.2.1",
		"203.0.113.9", "224.0.0.2", "240.0.0.1", "::1", "fc00::9",
		"fe80::1234", "::ffff:10.0.0.1",
	}
	for _, s := range private {
		if !isPrivateIP(net.ParseIP(s)) {
			t.Errorf("isPrivateIP(%s) = false, want true", s)
		}
	}

	for _, s := range []string{"8.8.8.8", "93.184.216.34", "1.1.1.1", "2606:4700:4700::1111"} {
		if isPrivateIP(net.ParseIP(s)) {
			t.Errorf("isPrivateIP(%s) = true, want false", s)
		}
	}
}
