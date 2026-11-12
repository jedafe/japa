package email

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubProvider is a controllable EmailProvider for dispatcher tests.
type stubProvider struct {
	name string
	err  error
}

func (s stubProvider) Name() string { return s.name }

func (s stubProvider) Send(_ context.Context, _ EmailRequest) (EmailResponse, error) {
	if s.err != nil {
		return EmailResponse{}, s.err
	}

	return EmailResponse{ProviderRef: "ref-" + s.name, Status: "success"}, nil
}

func TestDispatchFallsThroughOnFailure(t *testing.T) {
	t.Parallel()

	d := NewEmailDispatcher()
	d.RegisterProvider("transactional", stubProvider{name: "bad", err: errors.New("boom")})
	d.RegisterProvider("transactional", stubProvider{name: "good"})

	resp, err := d.Dispatch(context.Background(), EmailRequest{
		To: "a@b.c", Subject: "s", Body: "b", Category: "transactional",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if resp.ProviderRef != "ref-good" {
		t.Fatalf("expected fallback provider to serve, got %q", resp.ProviderRef)
	}
}

func TestDispatchNoProvidersReturnsSentinel(t *testing.T) {
	t.Parallel()

	d := NewEmailDispatcher()

	_, err := d.Dispatch(context.Background(), EmailRequest{
		To: "a@b.c", Subject: "s", Body: "b", Category: "transactional",
	})
	if !errors.Is(err, ErrNoProviders) {
		t.Fatalf("expected ErrNoProviders, got %v", err)
	}
}

func TestReplaceProvidersSwapsAtomically(t *testing.T) {
	t.Parallel()

	d := NewEmailDispatcher()
	d.RegisterProvider("transactional", stubProvider{name: "stale"})

	// Swapping in a map without the category must actually unregister the
	// previously registered provider — the stale-registration regression.
	d.ReplaceProviders(map[string][]EmailProvider{})

	if d.HasCategory("transactional") {
		t.Fatal("expected transactional chain to be empty after ReplaceProviders")
	}

	_, err := d.Dispatch(context.Background(), EmailRequest{
		To: "a@b.c", Subject: "s", Body: "b", Category: "transactional",
	})
	if !errors.Is(err, ErrNoProviders) {
		t.Fatalf("expected ErrNoProviders after swap, got %v", err)
	}

	d.ReplaceProviders(map[string][]EmailProvider{
		"transactional": {stubProvider{name: "fresh"}},
	})

	resp, err := d.Dispatch(context.Background(), EmailRequest{
		To: "a@b.c", Subject: "s", Body: "b", Category: "transactional",
	})
	if err != nil {
		t.Fatalf("dispatch after re-swap: %v", err)
	}

	if resp.ProviderRef != "ref-fresh" {
		t.Fatalf("expected fresh provider, got %q", resp.ProviderRef)
	}
}

func TestRequiredConfigFieldsCoverSupportedTypes(t *testing.T) {
	t.Parallel()

	for _, pt := range SupportedProviderTypes() {
		if RequiredConfigFields(pt) == nil {
			t.Fatalf("provider_type %q has no required-fields entry", pt)
		}
	}
}

func TestValidateProviderConfig(t *testing.T) {
	t.Parallel()

	base := ProviderConfig{
		Name:         "p",
		ProviderType: "resend",
		Config:       map[string]string{"from_address": "a@b.c"},
		Categories:   []string{"transactional"},
	}

	tests := []struct {
		name    string
		mutate  func(*ProviderConfig)
		secret  string
		wantErr bool
	}{
		{name: "valid resend", secret: "key"},
		{name: "missing name", mutate: func(c *ProviderConfig) { c.Name = " " }, secret: "key", wantErr: true},
		{name: "bad type", mutate: func(c *ProviderConfig) { c.ProviderType = "nope" }, secret: "key", wantErr: true},
		{name: "no categories", mutate: func(c *ProviderConfig) { c.Categories = nil }, secret: "key", wantErr: true},
		{name: "bad category", mutate: func(c *ProviderConfig) { c.Categories = []string{"nope"} }, secret: "key", wantErr: true},
		{name: "bad base_url", mutate: func(c *ProviderConfig) { c.BaseURL = "http://10.0.0.1" }, secret: "key", wantErr: true},
		{name: "missing from_address", mutate: func(c *ProviderConfig) { c.Config = map[string]string{} }, secret: "key", wantErr: true},
		{name: "missing secret", wantErr: true},
		{
			name: "smtp host only fails without from/username",
			mutate: func(c *ProviderConfig) {
				c.ProviderType = "smtp"
				c.Config = map[string]string{"host": "smtp.example.com"}
			},
			wantErr: true,
		},
		{
			name: "smtp with username fallback ok",
			mutate: func(c *ProviderConfig) {
				c.ProviderType = "smtp"
				c.Config = map[string]string{"host": "smtp.example.com", "username": "u@x.com"}
			},
		},
		{
			name: "smtp passwordless ok",
			mutate: func(c *ProviderConfig) {
				c.ProviderType = "smtp"
				c.Config = map[string]string{"host": "smtp.example.com", "from_address": "a@b.c"}
			},
		},
		{
			name:   "loopback http base_url ok",
			mutate: func(c *ProviderConfig) { c.BaseURL = "http://127.0.0.1:9000" },
			secret: "key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := base
			cfg.Config = map[string]string{"from_address": "a@b.c"}
			cfg.Categories = []string{"transactional"}

			if tt.mutate != nil {
				tt.mutate(&cfg)
			}

			err := ValidateProviderConfig(cfg, tt.secret)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestMaskSecret(t *testing.T) {
	t.Parallel()

	if got := maskSecret(""); got != "" {
		t.Fatalf("empty secret: got %q", got)
	}

	if got := maskSecret("short"); got != "****" {
		t.Fatalf("short secret: got %q", got)
	}

	if got := maskSecret("abcdefghijklmnop"); got != "abcd****mnop" {
		t.Fatalf("long secret: got %q", got)
	}
}

func TestBuildMIMEMessageSanitizesAndOrdersHeaders(t *testing.T) {
	t.Parallel()

	msg := string(buildMIMEMessage("f@x.com", "t@x.com", "hi\r\nBcc: evil@x.com", "body", false,
		map[string]string{
			"List-Unsubscribe": "<https://x.com/u>",
			"Bad Header":       "skip me", // space in name → dropped
		}))

	// The injected \r\n must be stripped — the smuggled Bcc text may remain
	// inside the single-line Subject value but must not become its own
	// header line.
	for _, line := range strings.Split(msg, "\r\n") {
		if strings.HasPrefix(line, "Bcc:") {
			t.Fatalf("header injection: smuggled Bcc line present:\n%s", msg)
		}
	}

	if strings.Contains(msg, "Bad Header:") {
		t.Fatalf("invalid header name was not dropped:\n%s", msg)
	}

	if !strings.Contains(msg, "List-Unsubscribe: <https://x.com/u>") {
		t.Fatalf("custom header missing:\n%s", msg)
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
		{"127.0.0.1", false},      // loopback exempt — dev relay
		{"::1", false},            // loopback exempt
		{"10.0.0.5", true},        // private
		{"172.16.0.1", true},      // private
		{"192.168.1.1", true},     // private
		{"169.254.169.254", true}, // link-local cloud metadata
		{"0.0.0.0", true},         // unspecified
		{"224.0.0.1", true},       // multicast
		{"100.64.0.1", false},     // CGNAT is neither private nor link-local per net.IP
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

func TestPostJSONAllowsLoopbackThroughGuardedDialer(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	status, body, err := postJSON(context.Background(), srv.URL, nil, map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("postJSON to loopback test server: %v", err)
	}

	if status != http.StatusOK || string(body) != `{"ok":true}` {
		t.Fatalf("unexpected response: %d %s", status, body)
	}
}
