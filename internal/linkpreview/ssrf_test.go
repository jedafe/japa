package linkpreview

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"::1", true},
		{"2001:4860:4860::8888", false},
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Errorf("failed to parse IP: %s", tt.ip)
			continue
		}
		got := IsPrivateIP(ip)
		if got != tt.want {
			t.Errorf("IsPrivateIP(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"https://example.com", false},
		{"http://example.com", false},
		{"ftp://example.com", true},
		{"file:///etc/passwd", true},
		{"http://127.0.0.1", true},
		{"http://10.0.0.1", true},
		{"http://192.168.1.1", true},
		{"", true},
	}
	for _, tt := range tests {
		err := ValidateURL(tt.url)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
		}
	}
}

func TestValidateRedirectURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid public redirect", "https://example.com/next", false},
		{"valid http redirect", "http://example.com/next", false},
		{"ftp scheme blocked", "ftp://example.com/next", true},
		{"file scheme blocked", "file:///etc/passwd", true},
		{"loopback IP blocked", "http://127.0.0.1/admin", true},
		{"class A private blocked", "http://10.0.0.1/internal", true},
		{"class C private blocked", "http://192.168.1.1/router", true},
		{"docker bridge blocked", "http://172.17.0.1/containers", true},
		{"link-local blocked", "http://169.254.1.1/metadata", true},
		{"empty URL", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRedirectURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRedirectURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestIsBlockedDomain(t *testing.T) {
	blacklist := []string{"evil.com", "bad.org"}

	tests := []struct {
		host string
		want bool
	}{
		{"evil.com", true},
		{"sub.evil.com", true},
		{"good.com", false},
		{"example.org", false},
		{"bad.org", true},
	}
	for _, tt := range tests {
		got := IsBlockedDomain(tt.host, blacklist)
		if got != tt.want {
			t.Errorf("IsBlockedDomain(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestSSRFRedirectBlocking(t *testing.T) {
	// Server that redirects to a private IP (simulates an open redirect to internal host).
	privateTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `<html><head><meta property="og:title" content="Internal"></head></html>`)
	}))
	defer privateTarget.Close()

	// Server that redirects to the private target server.
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, privateTarget.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	t.Run("redirect to private IP is blocked", func(t *testing.T) {
		fetcher := NewFetcher(nil, nil)
		_, err := fetcher.Fetch(t.Context(), redirectServer.URL)
		if err == nil {
			t.Error("expected error for redirect to private IP, got nil")
		}
	})
}

func TestMaxRedirectDepth(t *testing.T) {
	// Create a chain of redirects that exceeds maxRedirectDepth.
	var redirectChain []*httptest.Server
	for i := 0; i < maxRedirectDepth+2; i++ {
		idx := i
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if idx < len(redirectChain)-1 {
				http.Redirect(w, r, redirectChain[idx+1].URL, http.StatusFound)
			} else {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprintln(w, `<html><head><meta property="og:title" content="End"></head></html>`)
			}
		}))
		redirectChain = append(redirectChain, server)
	}
	for _, s := range redirectChain {
		defer s.Close()
	}

	fetcher := NewFetcher(nil, nil)
	_, err := fetcher.Fetch(t.Context(), redirectChain[0].URL)
	if err == nil {
		t.Error("expected error for too many redirects, got nil")
	}
}
