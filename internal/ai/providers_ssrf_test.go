// Package ai — providers_ssrf_test.go locks in the SSRF defense-in-depth
// validation added to NewProviderFromConfig. The admin-supplied BaseURL for
// every provider type that accepts one (openai, anthropic, devin, cursor)
// must be validated by validateEndpointAllowHTTP before the provider is
// constructed, so a misconfigured or malicious base URL cannot redirect AI
// requests to internal services.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Security)
package ai

import (
	"strings"
	"testing"
)

func TestNewProviderFromConfig_SSRF_RejectsInternalHTTPS(t *testing.T) {
	// HTTPS to an internal/loopback address must be rejected for every
	// provider type that accepts an admin-supplied base URL.
	internalURLs := []string{
		"https://127.0.0.1/v1",
		"https://localhost/v1",
		"https://169.254.169.254/v1", // cloud metadata endpoint
		"https://10.0.0.1/v1",
		"https://192.168.1.1/v1",
	}
	providerTypes := []string{"openai", "anthropic", "devin", "cursor"}

	for _, pt := range providerTypes {
		for _, u := range internalURLs {
			cfg := ProviderConfig{
				ProviderType: pt,
				BaseURL:      u,
				Name:         "test-" + pt,
			}
			_, err := NewProviderFromConfig(cfg, "sk-test")
			if err == nil {
				t.Errorf("NewProviderFromConfig(type=%s, baseURL=%q) = nil error, want error (internal HTTPS must be rejected)", pt, u)
				continue
			}
			if !strings.Contains(err.Error(), "invalid base URL") {
				t.Errorf("NewProviderFromConfig(type=%s, baseURL=%q) error = %q, want error mentioning \"invalid base URL\"", pt, u, err.Error())
			}
		}
	}
}

func TestNewProviderFromConfig_SSRF_RejectsBadScheme(t *testing.T) {
	// Non-http(s) schemes must be rejected.
	badSchemes := []string{
		"file:///etc/passwd",
		"ftp://example.com/v1",
		"gopher://example.com/v1",
		"javascript:alert(1)",
	}
	for _, u := range badSchemes {
		cfg := ProviderConfig{
			ProviderType: "openai",
			BaseURL:      u,
			Name:         "test-bad-scheme",
		}
		_, err := NewProviderFromConfig(cfg, "sk-test")
		if err == nil {
			t.Errorf("NewProviderFromConfig(baseURL=%q) = nil error, want error (bad scheme must be rejected)", u)
		}
	}
}

func TestNewProviderFromConfig_SSRF_AllowsEmptyBaseURL(t *testing.T) {
	// An empty BaseURL must be allowed — the constructor falls back to a
	// hardcoded default HTTPS endpoint. This is the common case for admins
	// who select a provider type without overriding the endpoint.
	providerTypes := []string{"openai", "anthropic", "devin", "cursor"}
	for _, pt := range providerTypes {
		cfg := ProviderConfig{
			ProviderType: pt,
			BaseURL:      "", // empty → constructor default
			Name:         "test-default-" + pt,
		}
		_, err := NewProviderFromConfig(cfg, "sk-test")
		if err != nil {
			t.Errorf("NewProviderFromConfig(type=%s, baseURL=\"\") = %v, want nil (empty base URL must use constructor default)", pt, err)
		}
	}
}

func TestNewProviderFromConfig_SSRF_AllowsHTTPForLocalProxy(t *testing.T) {
	// HTTP to a local proxy (e.g. Ollama at http://localhost:11434/v1)
	// must be allowed — this is the documented development/local-LLM path.
	localURLs := []string{
		"http://localhost:11434/v1",
		"http://127.0.0.1:11434/v1",
	}
	for _, u := range localURLs {
		cfg := ProviderConfig{
			ProviderType: "openai",
			BaseURL:      u,
			Name:         "test-ollama",
		}
		_, err := NewProviderFromConfig(cfg, "sk-test")
		if err != nil {
			t.Errorf("NewProviderFromConfig(baseURL=%q) = %v, want nil (HTTP local proxy must be allowed)", u, err)
		}
	}
}

func TestNewProviderFromConfig_SSRF_AllowsPublicHTTPS(t *testing.T) {
	// HTTPS to a public host must be allowed.
	publicURLs := []string{
		"https://api.openai.com/v1",
		"https://api.anthropic.com/v1",
		"https://openrouter.ai/api/v1",
		"https://api.commandcode.ai/provider/v1",
	}
	for _, u := range publicURLs {
		cfg := ProviderConfig{
			ProviderType: "openai",
			BaseURL:      u,
			Name:         "test-public",
		}
		_, err := NewProviderFromConfig(cfg, "sk-test")
		if err != nil {
			t.Errorf("NewProviderFromConfig(baseURL=%q) = %v, want nil (public HTTPS must be allowed)", u, err)
		}
	}
}
