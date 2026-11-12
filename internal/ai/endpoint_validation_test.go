// Package ai — endpoint_validation_test.go covers the validateEndpoint
// and validateEndpointAllowHTTP functions that provide SSRF protection
// for all LLM provider HTTP calls (Phase 11 task 11.07).
package ai

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

func TestValidateEndpoint_HTTPS_Valid(t *testing.T) {
	t.Parallel()

	valid := []string{
		"https://api.openai.com/v1/chat/completions",
		"https://api.anthropic.com/v1/messages",
		"https://api.cursor.com/v1/agents",
		"https://api.devin.ai/v3/organizations/org-abc/sessions",
		"https://token-plan-cn.xiaomimimo.com/v1/chat/completions",
	}
	for _, url := range valid {
		t.Run(url, func(t *testing.T) {
			t.Parallel()

			if err := validateEndpoint(url); err != nil {
				t.Errorf("validateEndpoint(%q) = %v, want nil", url, err)
			}
		})
	}
}

func TestValidateEndpoint_HTTP_Rejected(t *testing.T) {
	t.Parallel()

	// validateEndpoint (production mode) must reject HTTP — only HTTPS
	// is allowed for production LLM provider calls.
	httpURLs := []string{
		"http://api.openai.com/v1/chat/completions",
		"http://localhost:11434/v1/chat/completions",
		"http://127.0.0.1:8080/v1/chat/completions",
	}
	for _, url := range httpURLs {
		t.Run(url, func(t *testing.T) {
			t.Parallel()

			if err := validateEndpoint(url); err == nil {
				t.Errorf("validateEndpoint(%q) = nil, want error (HTTP rejected in production mode)", url)
			}
		})
	}
}

func TestValidateEndpoint_InternalHosts_Rejected(t *testing.T) {
	t.Parallel()

	// HTTPS endpoints to internal/loopback hosts must be rejected to
	// prevent SSRF to internal services (e.g. cloud metadata endpoints).
	internal := []string{
		"https://127.0.0.1/v1/chat/completions",
		"https://localhost/v1/chat/completions",
		"https://10.0.0.1/v1/chat/completions",
		"https://172.16.0.1/v1/chat/completions",
		"https://192.168.1.1/v1/chat/completions",
		"https://169.254.169.254/latest/meta-data/", // cloud metadata
		"https://[::1]/v1/chat/completions",
	}
	for _, url := range internal {
		t.Run(url, func(t *testing.T) {
			t.Parallel()

			if err := validateEndpoint(url); err == nil {
				t.Errorf("validateEndpoint(%q) = nil, want error (internal host rejected)", url)
			}
		})
	}
}

func TestValidateEndpoint_Invalid_Rejected(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"",
		"   ",
		"not-a-url",
		"ftp://api.openai.com/v1",
		"://missing-scheme",
		"https://",
		"https:///path-only",
	}
	for _, url := range invalid {
		t.Run(url, func(t *testing.T) {
			t.Parallel()

			if err := validateEndpoint(url); err == nil {
				t.Errorf("validateEndpoint(%q) = nil, want error", url)
			}
		})
	}
}

func TestValidateEndpointAllowHTTP_HTTP_Loopback_Allowed(t *testing.T) {
	t.Parallel()

	// validateEndpointAllowHTTP permits HTTP only to loopback addresses
	// (localhost, 127.0.0.0/8, ::1) for local dev proxies (Ollama, local
	// test servers). Non-loopback internal hosts are rejected for SSRF
	// defense even with HTTP.
	valid := []string{
		"http://localhost:11434/v1/chat/completions",
		"http://127.0.0.1:8080/v1/chat/completions",
		"http://[::1]:8080/v1/chat/completions",
		"https://api.openai.com/v1/chat/completions",
	}
	for _, url := range valid {
		t.Run(url, func(t *testing.T) {
			t.Parallel()

			if err := validateEndpointAllowHTTP(url); err != nil {
				t.Errorf("validateEndpointAllowHTTP(%q) = %v, want nil", url, err)
			}
		})
	}
}

func TestValidateEndpointAllowHTTP_HTTP_NonLoopbackInternal_Rejected(t *testing.T) {
	t.Parallel()

	// HTTP to non-loopback internal hosts must be rejected even with
	// allowHTTP. This prevents SSRF to cloud metadata (169.254.169.254)
	// and internal services (10.x, 172.16.x, 192.168.x) via HTTP.
	internal := []string{
		"http://169.254.169.254/latest/meta-data/", // cloud metadata
		"http://10.0.0.1/v1/chat/completions",
		"http://172.16.0.1/v1/chat/completions",
		"http://192.168.1.100:11434/v1/chat/completions",
		"http://0.0.0.0/v1/chat/completions",
	}
	for _, url := range internal {
		t.Run(url, func(t *testing.T) {
			t.Parallel()

			if err := validateEndpointAllowHTTP(url); err == nil {
				t.Errorf("validateEndpointAllowHTTP(%q) = nil, want error (non-loopback internal HTTP rejected)", url)
			}
		})
	}
}

func TestValidateEndpointAllowHTTP_HTTPS_Internal_Rejected(t *testing.T) {
	t.Parallel()

	// Even with allowHTTP, HTTPS to internal hosts must be rejected.
	// This prevents an attacker from using HTTPS to bypass the SSRF
	// check.
	internal := []string{
		"https://127.0.0.1/v1/chat/completions",
		"https://localhost/v1/chat/completions",
		"https://169.254.169.254/latest/meta-data/",
	}
	for _, url := range internal {
		t.Run(url, func(t *testing.T) {
			t.Parallel()

			if err := validateEndpointAllowHTTP(url); err == nil {
				t.Errorf("validateEndpointAllowHTTP(%q) = nil, want error (HTTPS to internal host rejected)", url)
			}
		})
	}
}

func TestValidateEndpointAllowHTTP_Invalid_Rejected(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"",
		"   ",
		"not-a-url",
		"ftp://api.openai.com/v1",
		"://missing-scheme",
		"https://",
	}
	for _, url := range invalid {
		t.Run(url, func(t *testing.T) {
			t.Parallel()

			if err := validateEndpointAllowHTTP(url); err == nil {
				t.Errorf("validateEndpointAllowHTTP(%q) = nil, want error", url)
			}
		})
	}
}

func TestIsInternalHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		host string
		want bool
	}{
		// Loopback
		{"localhost", true},
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"::1", true},
		// Private ranges
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		// Link-local (cloud metadata)
		{"169.254.169.254", true},
		// Unspecified
		{"0.0.0.0", true},
		// Public hosts
		{"api.openai.com", false},
		{"api.anthropic.com", false},
		{"1.1.1.1", false},
		{"8.8.8.8", false},
		// .localhost TLD
		{"myapp.localhost", true},
	}
	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()

			if got := isInternalHost(tc.host); got != tc.want {
				t.Errorf("isInternalHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestIsLoopbackHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		host string
		want bool
	}{
		// Loopback
		{"localhost", true},
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"127.255.255.255", true},
		{"::1", true},
		{"myapp.localhost", true},
		// Non-loopback internal (must return false)
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false},
		{"0.0.0.0", false},
		// Public
		{"api.openai.com", false},
		{"1.1.1.1", false},
	}
	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()

			if got := isLoopbackHost(tc.host); got != tc.want {
				t.Errorf("isLoopbackHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestSafeInt32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   int
		want int32
	}{
		{"zero", 0, 0},
		{"positive", 42, 42},
		{"negative", -42, -42},
		{"max int32", 2147483647, 2147483647},
		{"overflow positive", 2147483648, 2147483647}, // math.MaxInt32
		{"large overflow", 9999999999, 2147483647},
		{"min int32", -2147483648, -2147483648},
		{"overflow negative", -2147483649, -2147483648}, // math.MinInt32
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := safeInt32(tc.in); got != tc.want {
				t.Errorf("safeInt32(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestSafeInt64ToInt32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   int64
		want int32
	}{
		{"zero", 0, 0},
		{"positive", 42, 42},
		{"negative", -42, -42},
		{"max int32", 2147483647, 2147483647},
		{"overflow positive", 2147483648, 2147483647},
		{"large overflow", 9999999999, 2147483647},
		{"min int32", -2147483648, -2147483648},
		{"overflow negative", -2147483649, -2147483648},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := safeInt64ToInt32(tc.in); got != tc.want {
				t.Errorf("safeInt64ToInt32(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsBlockedRemoteIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ip      string
		blocked bool
	}{
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"127.0.0.1", false}, // loopback exempt — Ollama dev proxy
		{"::1", false},       // loopback exempt
		{"10.0.0.5", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true}, // link-local cloud metadata
		{"100.64.0.1", true},      // CGNAT
		{"0.0.0.0", true},
		{"240.0.0.1", true},
		{"192.0.2.1", true},  // TEST-NET-1 documentation range
		{"198.18.0.1", true}, // benchmarking range
		{"224.0.0.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			t.Parallel()

			if got := isBlockedRemoteIP(net.ParseIP(tt.ip)); got != tt.blocked {
				t.Fatalf("isBlockedRemoteIP(%s) = %v, want %v", tt.ip, got, tt.blocked)
			}
		})
	}
}

func TestGuardedHTTPClient_AllowsLoopback(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	resp, err := guardedHTTPClient(5 * time.Second).Get(srv.URL)
	if err != nil {
		t.Fatalf("guarded client rejected loopback: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
}

func TestGuardedDialTLSBlocksLoopback(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "https://")

	// https+loopback must be rejected at dial — the URL policy forbids it,
	// and a DNS name resolving to loopback would otherwise slip past the
	// literal-host check.
	_, err := guardedDialTLSContext(context.Background(), "tcp", addr)
	if !errors.Is(err, errInvalidEndpoint) {
		t.Fatalf("expected errInvalidEndpoint for loopback TLS dial, got %v", err)
	}
}

func TestIsProxyEndpointAddr(t *testing.T) {
	// t.Setenv mutates process env — cannot run in parallel. Clear all
	// four vars first so a proxied test machine can't skew the results.
	for _, env := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
		t.Setenv(env, "")
	}

	if isProxyEndpointAddr("proxy.corp:3128") {
		t.Fatal("no proxy env: must not match")
	}

	t.Setenv("HTTPS_PROXY", "http://user:pass@proxy.corp:3128")

	if !isProxyEndpointAddr("proxy.corp:3128") {
		t.Fatal("https proxy host: expected match")
	}

	if !isProxyEndpointAddr("PROXY.CORP:3128") {
		t.Fatal("hostname match must be case-insensitive")
	}

	if isProxyEndpointAddr("target.corp:443") {
		t.Fatal("non-proxy addr must not match")
	}

	// A request with no scheme-matching proxy dials the target directly —
	// the guard must still apply even though some proxy env is set.
	if isProxyEndpointAddr("10.0.0.5:8080") {
		t.Fatal("direct-dialed target must not be treated as proxy")
	}

	// Same hostname on a different port is not proxy traffic — a service
	// colocated with the proxy must stay guarded.
	if isProxyEndpointAddr("proxy.corp:8443") {
		t.Fatal("port mismatch: same host on another port must not match")
	}

	// Scheme-default port: the transport dials the proxy host on the
	// scheme's default when the URL omits the port.
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("HTTP_PROXY", "http://proxy.corp")
	if !isProxyEndpointAddr("proxy.corp:80") {
		t.Fatal("omitted proxy port must match scheme default :80")
	}

	if isProxyEndpointAddr("proxy.corp:3128") {
		t.Fatal("default-port proxy must not match explicit port dials")
	}
}

func TestValidateProviderConfig(t *testing.T) {
	t.Parallel()

	base := ProviderConfig{
		Name:         "primary",
		ProviderType: "openai",
		BaseURL:      "https://api.openai.com/v1",
		Model:        "gpt-5.4",
		Categories:   []string{"chat"},
	}

	cases := []struct {
		name    string
		mutate  func(*ProviderConfig)
		apiKey  string
		wantErr string
	}{
		{"valid", func(*ProviderConfig) {}, "sk-test", ""},
		{"blank name", func(c *ProviderConfig) { c.Name = "  " }, "sk-test", "name is required"},
		{"bad type", func(c *ProviderConfig) { c.ProviderType = "bogus" }, "sk-test", "invalid provider_type"},
		{"no categories", func(c *ProviderConfig) { c.Categories = nil }, "sk-test", "at least one category"},
		{"bad category", func(c *ProviderConfig) { c.Categories = []string{"bogus"} }, "sk-test", "invalid category"},
		{"empty api key", func(*ProviderConfig) {}, "", "api_key is required"},
		{"public http base_url", func(c *ProviderConfig) { c.BaseURL = "http://api.openai.com" }, "sk-test", "invalid base_url"},
		{"private base_url", func(c *ProviderConfig) { c.BaseURL = "https://10.0.0.5" }, "sk-test", "invalid base_url"},
		{"loopback http base_url allowed", func(c *ProviderConfig) { c.BaseURL = "http://localhost:11434" }, "sk-test", ""},
		{"empty base_url allowed", func(c *ProviderConfig) { c.BaseURL = "" }, "sk-test", ""},
		{"decimal ip base_url rejected", func(c *ProviderConfig) { c.BaseURL = "https://2130706433" }, "sk-test", "invalid base_url"},
		{"model optional", func(c *ProviderConfig) { c.Model = "" }, "sk-test", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := base
			tc.mutate(&cfg)

			err := ValidateProviderConfig(cfg, tc.apiKey)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}

			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q missing %q", err, tc.wantErr)
			}
		})
	}
}

func TestMaskAPIKey(t *testing.T) {
	t.Parallel()

	if got := maskAPIKey(""); got != "" {
		t.Fatalf("empty key: got %q", got)
	}

	if got := maskAPIKey("123456789012"); got != "****" {
		t.Fatalf("12-char key must be fully masked: got %q", got)
	}

	if got := maskAPIKey("1234567890123"); got != "1234****0123" {
		t.Fatalf("13-char key: got %q", got)
	}
}
