// Package auth — HaveIBeenPwned (HIBP) password breach detection.
//
// Uses the k-Anonymity model: only a SHA-1 prefix (5 hex chars) is sent
// to the HIBP API. The full hash never leaves the server, so the
// password remains private even in transit to the third-party service.
package auth

import (
	"bufio"
	"crypto/sha1" // #nosec G401 G505 -- SHA-1 required by HIBP k-Anonymity API; cannot use SHA-256
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// BreachedError is returned when a password appears in known data breaches.
type BreachedError struct {
	Count int
}

func (e *BreachedError) Error() string {
	return fmt.Sprintf("password found in %d data breaches", e.Count)
}

// BreachCheckError signals that the breach check itself failed (not that
// the password is breached). Callers should treat this as a non-blocking
// advisory — never reject a password on a failed check.
type BreachCheckError struct {
	Err error
}

func (e *BreachCheckError) Error() string {
	return fmt.Sprintf("breach check failed: %v", e.Err)
}

func (e *BreachCheckError) Unwrap() error { return e.Err }

// breachCacheEntry holds a cached breach-check result with an expiry time.
type breachCacheEntry struct {
	isBreached bool
	count      int
	expiresAt  time.Time
}

// breachCache is a simple in-memory cache keyed by SHA-1 hex digest of
// the password. Entries expire after 1 hour.
type breachCache struct {
	mu      sync.RWMutex
	entries map[string]breachCacheEntry
	ttl     time.Duration
}

func newBreachCache(ttl time.Duration) *breachCache {
	return &breachCache{
		entries: make(map[string]breachCacheEntry),
		ttl:     ttl,
	}
}

func (c *breachCache) get(hash string) (isBreached bool, count int, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.entries[hash]
	if !found || time.Now().After(entry.expiresAt) {
		return false, 0, false
	}

	return entry.isBreached, entry.count, true
}

func (c *breachCache) set(hash string, isBreached bool, count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[hash] = breachCacheEntry{
		isBreached: isBreached,
		count:      count,
		expiresAt:  time.Now().Add(c.ttl),
	}
}

// global breach cache — lives for the process lifetime.
var globalBreachCache = newBreachCache(1 * time.Hour)

// breachRateLimiter enforces a minimum interval between HIBP API calls
// to avoid hitting the service's rate limits (1 request/second).
type breachRateLimiter struct {
	mu        sync.Mutex
	lastCheck time.Time
	interval  time.Duration
}

func newBreachRateLimiter(minInterval time.Duration) *breachRateLimiter {
	return &breachRateLimiter{interval: minInterval}
}

// wait blocks until at least minInterval has elapsed since the last call,
// then records the current time. This is a simple token-bucket-free way
// to stay under HIBP's 1 req/s limit.
func (r *breachRateLimiter) wait() {
	r.mu.Lock()
	defer r.mu.Unlock()

	elapsed := time.Since(r.lastCheck)
	if elapsed < r.interval {
		time.Sleep(r.interval - elapsed)
	}

	r.lastCheck = time.Now()
}

var globalBreachLimiter = newBreachRateLimiter(1 * time.Second)

// hibpClient is the HTTP client used for HIBP API calls. Extracted
// as a package-level variable so tests can swap in a test server.
var hibpClient = &http.Client{
	Timeout: 5 * time.Second,
}

var hibpRangeURL = "https://api.pwnedpasswords.com/range/"

// CheckPasswordBreach checks whether a password has appeared in known
// data breaches via the HaveIBeenPwned Pwned Passwords API.
//
// It uses the k-Anonymity model: only the first 5 hex characters of the
// password's SHA-1 hash are sent to HIBP. The remaining suffix is matched
// locally. This means HIBP never sees the full hash.
//
// Returns:
//   - isBreached: true if the password was found in at least one breach
//   - count: the number of times the password appeared across breaches (0 if not found)
//   - error: non-nil only if the check itself failed (network, rate limit, etc.)
//     When error is non-nil, isBreached and count are zero-valued and
//     callers should degrade gracefully.
func CheckPasswordBreach(password string) (isBreached bool, count int, err error) {
	// Guard: empty password cannot be checked (should be caught earlier by policy).
	if password == "" {
		return false, 0, errors.New("cannot check breach for empty password")
	}

	// Compute SHA-1 hash of the plaintext password.
	hash := sha1Hash(password)
	prefix := strings.ToUpper(hash[:5])
	suffix := strings.ToUpper(hash[5:])

	// Check the in-memory cache first.
	if cached, cachedCount, ok := globalBreachCache.get(hash); ok {
		return cached, cachedCount, nil
	}

	// Enforce rate limiting before hitting the API.
	globalBreachLimiter.wait()

	// Fetch the range from HIBP — single round-trip.
	isBreached, count, err = queryHIBPPrefixRange(prefix, suffix)
	if err != nil {
		// Graceful degradation: if HIBP is unreachable, log the error
		// and allow the password. Never block registration on a third-party
		// service failure.
		return false, 0, &BreachCheckError{Err: err}
	}

	// Cache the result for 1 hour.
	globalBreachCache.set(hash, isBreached, count)

	return isBreached, count, nil
}

// sha1Hash returns the lowercase hex-encoded SHA-1 digest of the input string.
func sha1Hash(s string) string {
	h := sha1.New() // #nosec G401 -- SHA-1 required by HIBP k-Anonymity API
	h.Write([]byte(s))

	return hex.EncodeToString(h.Sum(nil))
}

// queryHIBPPrefixRange queries the HIBP Pwned Passwords API for a given
// 5-char SHA-1 prefix and checks whether the suffix appears in the
// response. Returns the breach count for the suffix if found, 0 otherwise.
func queryHIBPPrefixRange(prefix, suffix string) (bool, int, error) {
	resp, err := hibpClient.Get(hibpRangeURL + prefix)
	if err != nil {
		return false, 0, fmt.Errorf("hibp request failed: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		// No passwords match this prefix at all — definitely not breached.
		return false, 0, nil
	}

	if resp.StatusCode != http.StatusOK {
		return false, 0, fmt.Errorf("hibp returned status %d", resp.StatusCode)
	}

	// Scan line-by-line. Each line is "SUFFIX:COUNT".
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		if strings.ToUpper(parts[0]) == suffix {
			var n int
			if _, parseErr := fmt.Sscanf(parts[1], "%d", &n); parseErr == nil {
				return true, n, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return false, 0, fmt.Errorf("reading hibp response: %w", err)
	}

	// Suffix not found in the response — password is not breached.
	return false, 0, nil
}

// ResetBreachCache clears the in-memory breach cache. Intended for tests.
func ResetBreachCache() {
	globalBreachCache.mu.Lock()
	defer globalBreachCache.mu.Unlock()
	globalBreachCache.entries = make(map[string]breachCacheEntry)
}

// SetHIBPClient replaces the HTTP client used for HIBP API calls.
// Intended for tests to inject a test server.
func SetHIBPClient(c *http.Client) {
	hibpClient = c
}

// SetHIBPRangeURL replaces the base URL for the HIBP range API.
// Intended for tests to redirect requests to a test server.
func SetHIBPRangeURL(url string) {
	hibpRangeURL = url
}
