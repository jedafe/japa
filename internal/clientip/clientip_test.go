package clientip

import (
	"context"
	"net/http"
	"testing"
)

const testClientIP = "1.2.3.4"

func requestWith(remoteAddr string, headers map[string]string) *http.Request {
	r, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr

	for k, v := range headers {
		r.Header.Set(k, v)
	}

	return r
}

func TestFromRequest_IgnoresSpoofedHeadersFromUntrustedPeer(t *testing.T) {
	t.Parallel()

	r := requestWith("203.0.113.10:4321", map[string]string{
		"X-Forwarded-For": testClientIP,
		"X-Real-IP":       "5.6.7.8",
	})

	if got := FromRequest(r); got != "203.0.113.10" {
		t.Fatalf("expected peer address, got %q", got)
	}
}

func TestFromRequest_TrustsProxyForwardedFor(t *testing.T) {
	t.Parallel()

	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-For": testClientIP + ", 10.0.0.5",
	})

	if got := FromRequest(r); got != testClientIP {
		t.Fatalf("expected forwarded client address, got %q", got)
	}
}

func TestFromRequest_TrustsProxyRealIP(t *testing.T) {
	t.Parallel()

	r := requestWith("127.0.0.1:4321", map[string]string{"X-Real-IP": testClientIP})

	if got := FromRequest(r); got != testClientIP {
		t.Fatalf("expected X-Real-IP value, got %q", got)
	}
}

func TestFromRequest_NoHeaders(t *testing.T) {
	t.Parallel()

	if got := FromRequest(requestWith("198.51.100.7:1111", nil)); got != "198.51.100.7" {
		t.Fatalf("expected peer address, got %q", got)
	}
}
