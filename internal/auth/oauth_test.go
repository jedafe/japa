package auth

import (
	"testing"
)

func TestOAuthConfig_SupportedProviders(t *testing.T) {
	tests := []struct {
		name     string
		config   OAuthConfig
		expected []string
	}{
		{
			name:     "no providers configured",
			config:   OAuthConfig{},
			expected: nil,
		},
		{
			name: "google only",
			config: OAuthConfig{
				GoogleClientID:     "google-id",
				GoogleClientSecret: "google-secret",
			},
			expected: []string{"google"},
		},
		{
			name: "github only",
			config: OAuthConfig{
				GitHubClientID:     "github-id",
				GitHubClientSecret: "github-secret",
			},
			expected: []string{"github"},
		},
		{
			name: "both providers",
			config: OAuthConfig{
				GoogleClientID:     "google-id",
				GoogleClientSecret: "google-secret",
				GitHubClientID:     "github-id",
				GitHubClientSecret: "github-secret",
			},
			expected: []string{"google", "github"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.SupportedProviders()
			if len(result) != len(tt.expected) {
				t.Errorf("got %d providers, want %d", len(result), len(tt.expected))
				return
			}
			for i, p := range result {
				if p != tt.expected[i] {
					t.Errorf("provider[%d] = %q, want %q", i, p, tt.expected[i])
				}
			}
		})
	}
}

func TestOAuthConfig_NewProvider(t *testing.T) {
	cfg := OAuthConfig{
		GoogleClientID:     "google-id",
		GoogleClientSecret: "google-secret",
		GitHubClientID:     "github-id",
		GitHubClientSecret: "github-secret",
		RedirectBase:       "http://localhost:8080",
	}

	t.Run("google provider", func(t *testing.T) {
		p := cfg.NewProvider("google")
		if p == nil {
			t.Fatal("expected non-nil provider")
		}
		if p.Name() != "google" {
			t.Errorf("Name() = %q, want %q", p.Name(), "google")
		}
	})

	t.Run("github provider", func(t *testing.T) {
		p := cfg.NewProvider("github")
		if p == nil {
			t.Fatal("expected non-nil provider")
		}
		if p.Name() != "github" {
			t.Errorf("Name() = %q, want %q", p.Name(), "github")
		}
	})

	t.Run("unsupported provider", func(t *testing.T) {
		p := cfg.NewProvider("twitter")
		if p != nil {
			t.Error("expected nil for unsupported provider")
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		p := cfg.NewProvider("Google")
		if p == nil {
			t.Fatal("expected non-nil for case-insensitive match")
		}
	})
}

func TestOAuthConfig_NewProvider_NotConfigured(t *testing.T) {
	cfg := OAuthConfig{} // empty config

	p := cfg.NewProvider("google")
	if p != nil {
		t.Error("expected nil when Google is not configured")
	}

	p = cfg.NewProvider("github")
	if p != nil {
		t.Error("expected nil when GitHub is not configured")
	}
}

func TestGenerateOAuthState(t *testing.T) {
	state1, err := GenerateOAuthState()
	if err != nil {
		t.Fatalf("GenerateOAuthState failed: %v", err)
	}
	if state1 == "" {
		t.Error("expected non-empty state")
	}

	state2, err := GenerateOAuthState()
	if err != nil {
		t.Fatalf("GenerateOAuthState failed: %v", err)
	}

	// Two states should be different (cryptographic randomness)
	if state1 == state2 {
		t.Error("expected unique states, got duplicate")
	}
}

func TestValidateOAuthState(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		received string
		want     bool
	}{
		{
			name:     "matching states",
			expected: "abc123",
			received: "abc123",
			want:     true,
		},
		{
			name:     "mismatched states",
			expected: "abc123",
			received: "xyz789",
			want:     false,
		},
		{
			name:     "empty expected",
			expected: "",
			received: "abc123",
			want:     false,
		},
		{
			name:     "empty received",
			expected: "abc123",
			received: "",
			want:     false,
		},
		{
			name:     "both empty",
			expected: "",
			received: "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateOAuthState(tt.expected, tt.received)
			if result != tt.want {
				t.Errorf("ValidateOAuthState(%q, %q) = %v, want %v",
					tt.expected, tt.received, result, tt.want)
			}
		})
	}
}

func TestGoogleProvider_AuthURL(t *testing.T) {
	p := &googleProvider{
		clientID:     "test-client-id",
		clientSecret: "test-secret",
		redirectURL:  "http://localhost:8080/callback",
	}

	url := p.AuthURL("test-state")
	if url == "" {
		t.Error("expected non-empty auth URL")
	}
	if !contains(url, "client_id=test-client-id") {
		t.Error("auth URL should contain client_id")
	}
	if !contains(url, "state=test-state") {
		t.Error("auth URL should contain state")
	}
	if !contains(url, "accounts.google.com") {
		t.Error("auth URL should point to Google")
	}
}

func TestGitHubProvider_AuthURL(t *testing.T) {
	p := &githubProvider{
		clientID:     "test-client-id",
		clientSecret: "test-secret",
		redirectURL:  "http://localhost:8080/callback",
	}

	url := p.AuthURL("test-state")
	if url == "" {
		t.Error("expected non-empty auth URL")
	}
	if !contains(url, "client_id=test-client-id") {
		t.Error("auth URL should contain client_id")
	}
	if !contains(url, "state=test-state") {
		t.Error("auth URL should contain state")
	}
	if !contains(url, "github.com") {
		t.Error("auth URL should point to GitHub")
	}
}

func TestTokenResponse(t *testing.T) {
	// Verify that the tokenResponse struct has the expected fields
	resp := tokenResponse{
		AccessToken: "test-token",
		TokenType:   "Bearer",
	}
	if resp.AccessToken != "test-token" {
		t.Errorf("AccessToken = %q, want %q", resp.AccessToken, "test-token")
	}
}

// contains is a simple string contains check for test assertions.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
