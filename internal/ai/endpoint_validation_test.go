// Package ai — endpoint_validation_test.go covers the validateEndpoint
// and validateEndpointAllowHTTP functions that provide SSRF protection
// for all LLM provider HTTP calls (Phase 11 task 11.07).
package ai

import "testing"

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
