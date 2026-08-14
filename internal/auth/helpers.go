package auth

import (
	"net/http"
	"strings"
)

// deviceName extracts a human-readable device label from the User-Agent
// header, truncated to 255 chars. Returns nil when the header is absent
// or empty so the refresh_tokens.device_name column stays NULL.
func deviceName(r *http.Request) *string {
	ua := strings.TrimSpace(r.UserAgent())
	if ua == "" {
		return nil
	}
	if len(ua) > 255 {
		ua = ua[:255]
	}
	return &ua
}

// ipPtr returns a pointer to the client IP, or nil when empty. Uses the
// trusted-proxy-aware clientIP resolver so the stored value cannot be
// spoofed via X-Forwarded-For from an untrusted peer.
func ipPtr(r *http.Request) *string {
	ip := clientIP(r)
	if ip == "" {
		return nil
	}
	return &ip
}
