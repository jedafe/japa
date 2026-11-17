package linkpreview

import (
	"net"
	"strings"

	"github.com/jedafe/japa/internal/ssrfguard"
)

// The concrete SSRF primitives (IP ranges, URL/redirect validation, the
// dial-time-guarded http.Client) live in internal/ssrfguard — a leaf
// package so the Asynq worker and other fetch paths can share them
// without importing this package's handler layer. These wrappers keep
// the package's existing API stable.

// IsPrivateIP returns true if the given IP address is in a private/reserved range.
func IsPrivateIP(ip net.IP) bool {
	return ssrfguard.IsPrivateIP(ip)
}

// ValidateURL checks whether a URL is safe to fetch (not SSRF).
func ValidateURL(rawURL string) error {
	return ssrfguard.ValidateURL(rawURL)
}

// ValidateRedirectURL checks whether a redirect target URL is safe.
func ValidateRedirectURL(urlStr string) error {
	return ssrfguard.ValidateRedirectURL(urlStr)
}

// IsBlockedDomain checks whether a domain is in the provided blacklist.
func IsBlockedDomain(host string, blacklist []string) bool {
	host = strings.ToLower(host)

	for _, blocked := range blacklist {
		blocked = strings.ToLower(blocked)
		if host == blocked || strings.HasSuffix(host, "."+blocked) {
			return true
		}
	}

	return false
}
