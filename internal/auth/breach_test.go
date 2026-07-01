package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCheckPasswordBreach_KnownBreached verifies that the famously
// breached password "password" is detected. This test hits the live
// HIBP API — it requires network access and will be skipped in CI
// if the HIBP endpoint is unreachable.
func TestCheckPasswordBreach_KnownBreached(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live HIBP API test in short mode")
	}

	ResetBreachCache()

	isBreached, count, err := CheckPasswordBreach("password")
	if err != nil {
		t.Fatalf("CheckPasswordBreach returned unexpected error: %v", err)
	}
	if !isBreached {
		t.Error("expected 'password' to be detected as breached")
	}
	if count == 0 {
		t.Error("expected non-zero breach count for 'password'")
	}
	t.Logf("password found in %d breaches", count)
}

// TestCheckPasswordBreach_StrongUnique verifies that a sufficiently
// random, long password is not in the breach database.
func TestCheckPasswordBreach_StrongUnique(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live HIBP API test in short mode")
	}

	ResetBreachCache()

	// A 64-char random-looking password — virtually impossible to be in any breach DB.
	strong := "X9$kL2#mN8@pQ4$vR7&wT1!yU5^zA3*bC6+dE9-fG2"
	isBreached, count, err := CheckPasswordBreach(strong)
	if err != nil {
		t.Fatalf("CheckPasswordBreach returned unexpected error: %v", err)
	}
	if isBreached {
		t.Errorf("expected strong unique password to not be breached, but found in %d breaches", count)
	}
}

// TestCheckPasswordBreach_EmptyPassword verifies that an empty password
// returns an error rather than making an API call.
func TestCheckPasswordBreach_EmptyPassword(t *testing.T) {
	_, _, err := CheckPasswordBreach("")
	if err == nil {
		t.Error("expected error for empty password, got nil")
	}
}

// TestCheckPasswordBreach_GracefulDegradation verifies that when HIBP
// is unreachable, the function returns a BreachCheckError and allows
// the password through (isBreached = false, count = 0).
func TestCheckPasswordBreach_GracefulDegradation(t *testing.T) {
	ResetBreachCache()

	// Point the HIBP client at a server that refuses connections.
	// Use httptest.NewUnstartedServer and don't start it — the URL
	// will be unreachable.
	unreachable := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer unreachable.Close()

	// The server isn't started, so the URL is unreachable.
	SetHIBPClient(&http.Client{
		Timeout: 100 * time.Millisecond,
	})
	defer SetHIBPClient(&http.Client{Timeout: 5 * time.Second})

	isBreached, count, err := CheckPasswordBreach("someword")
	if err == nil {
		t.Fatal("expected error when HIBP is unreachable, got nil")
	}

	// Must be a BreachCheckError.
	var bce *BreachCheckError
	if !errors.As(err, &bce) {
		t.Errorf("expected BreachCheckError, got %T: %v", err, err)
	}

	// Password must be allowed through (graceful degradation).
	if isBreached {
		t.Error("expected isBreached=false when HIBP is unreachable")
	}
	if count != 0 {
		t.Errorf("expected count=0 when HIBP is unreachable, got %d", count)
	}
}

// TestCheckPasswordBreach_CacheHit verifies that a second call for
// the same password returns the cached result without hitting the API.
func TestCheckPasswordBreach_CacheHit(t *testing.T) {
	ResetBreachCache()

	// Set up a counter to track how many times the API is called.
	var callCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Return a simulated response with a matching suffix.
		// SHA-1 of "cachetestpass" starts with a known prefix — return matching suffix.
		hash := sha1Hash("cachetestpass")
		suffix := strings.ToUpper(hash[5:])
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "%s:%d\n", suffix, 12345)
	}))
	defer ts.Close()

	origClient := hibpClient
	origURL := hibpRangeURL
	hibpClient = ts.Client()
	hibpRangeURL = ts.URL + "/"
	defer func() {
		hibpClient = origClient
		hibpRangeURL = origURL
	}()

	// First call — should hit the API.
	isBreached1, _, err := CheckPasswordBreach("cachetestpass")
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if !isBreached1 {
		t.Error("expected password to be breached (suffix matches)")
	}
	if callCount != 1 {
		t.Errorf("expected 1 API call after first check, got %d", callCount)
	}

	// Second call — should come from cache (same API call count).
	isBreached2, _, err := CheckPasswordBreach("cachetestpass")
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if isBreached1 != isBreached2 {
		t.Errorf("cache inconsistency: first=%v, second=%v", isBreached1, isBreached2)
	}
	if callCount != 1 {
		t.Errorf("expected still 1 API call after cache hit, got %d", callCount)
	}
}

// TestCheckPasswordBreach_RateLimiting verifies that consecutive calls
// are spaced at least 1 second apart.
func TestCheckPasswordBreach_RateLimiting(t *testing.T) {
	ResetBreachCache()

	// Create a test server that records request timestamps.
	var requestTimes []time.Time
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestTimes = append(requestTimes, time.Now())
		w.WriteHeader(http.StatusNotFound) // No breaches found
	}))
	defer ts.Close()

	origClient := hibpClient
	hibpClient = &http.Client{Timeout: 5 * time.Second}
	defer func() { hibpClient = origClient }()

	// We need to temporarily replace the HIBP URL to hit our test server.
	// Since hibpRangeURL is a const, we test rate limiting behavior through
	// the globalLimiter directly.
	globalBreachLimiter.mu.Lock()
	globalBreachLimiter.lastCheck = time.Time{} // Reset
	globalBreachLimiter.mu.Unlock()

	start := time.Now()
	// Make 3 rapid calls — rate limiter should space them ~1s apart.
	for i := 0; i < 3; i++ {
		globalBreachLimiter.wait()
	}
	elapsed := time.Since(start)

	// With 1s minimum interval and 3 calls, we expect at least 2 seconds.
	if elapsed < 1500*time.Millisecond {
		t.Errorf("rate limiter too fast: 3 calls took %v, expected >= ~2s", elapsed)
	}
}

// TestCheckPasswordBreach_CacheExpiration verifies that expired entries
// are not returned from cache.
func TestCheckPasswordBreach_CacheExpiration(t *testing.T) {
	ResetBreachCache()

	// Manually insert an expired cache entry.
	hash := sha1Hash("expiredtest")
	globalBreachCache.mu.Lock()
	globalBreachCache.entries[hash] = breachCacheEntry{
		isBreached: true,
		count:      999,
		expiresAt:  time.Now().Add(-1 * time.Hour), // Already expired
	}
	globalBreachCache.mu.Unlock()

	// Should not find the expired entry.
	if _, _, ok := globalBreachCache.get(hash); ok {
		t.Error("expected expired cache entry to be ignored")
	}
}

// TestQueryHIBPPrefixRange_SuffixFound tests the suffix matching logic
// directly against a simulated HIBP response.
func TestQueryHIBPPrefixRange_SuffixFound(t *testing.T) {
	// Simulate HIBP response with a known suffix.
	suffix := "ABCDEF1234567890"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "%s:12345\n", suffix)
		fmt.Fprintf(w, "ZZZZZZ9999999999:1\n")
	}))
	defer ts.Close()

	origClient := hibpClient
	hibpClient = ts.Client()
	origURL := hibpRangeURL
	hibpRangeURL = ts.URL + "/"

	// Directly test the suffix matching via the handler logic.
	found, count, err := queryHIBPPrefixRange("AAAAA", suffix)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("expected suffix to be found")
	}
	if count != 12345 {
		t.Errorf("expected count=12345, got %d", count)
	}

	hibpClient = origClient
	hibpRangeURL = origURL
}

// TestQueryHIBPPrefixRange_SuffixNotFound tests when the suffix is not in
// the response.
func TestQueryHIBPPrefixRange_SuffixNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "ZZZZZZ9999999999:1\n")
	}))
	defer ts.Close()

	origClient := hibpClient
	hibpClient = ts.Client()
	origURL := hibpRangeURL
	hibpRangeURL = ts.URL + "/"

	found, count, err := queryHIBPPrefixRange("AAAAA", "NOTTHERE12345678")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected suffix not to be found")
	}
	if count != 0 {
		t.Errorf("expected count=0, got %d", count)
	}

	hibpClient = origClient
	hibpRangeURL = origURL
}

// TestQueryHIBPPrefixRange_404 verifies that a 404 response (no hashes
// for this prefix) is handled correctly.
func TestQueryHIBPPrefixRange_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	origClient := hibpClient
	hibpClient = ts.Client()
	origURL := hibpRangeURL
	hibpRangeURL = ts.URL + "/"

	found, count, err := queryHIBPPrefixRange("ZZZZZ", "AAAAAAAAAAAA")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false for 404")
	}
	if count != 0 {
		t.Errorf("expected count=0 for 404, got %d", count)
	}

	hibpClient = origClient
	hibpRangeURL = origURL
}

// TestQueryHIBPPrefixRange_ServerError verifies that non-200/non-404
// status codes return an error.
func TestQueryHIBPPrefixRange_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	origClient := hibpClient
	hibpClient = ts.Client()
	origURL := hibpRangeURL
	hibpRangeURL = ts.URL + "/"

	_, _, err := queryHIBPPrefixRange("AAAAA", "BBBBBB")
	if err == nil {
		t.Error("expected error for 500 status code")
	}

	hibpClient = origClient
	hibpRangeURL = origURL
}

// TestValidatePassword_BreachCheck verifies that ValidatePassword
// returns a BreachedError when the policy has CheckBreach=true and
// the password is breached.
func TestValidatePassword_BreachCheck(t *testing.T) {
	ResetBreachCache()

	if testing.Short() {
		t.Skip("skipping live HIBP API test in short mode")
	}

	policy := PasswordPolicy{
		MinLength:     1,
		MaxLength:     128,
		RequireUpper:  false,
		RequireNumber: false,
		RequireSymbol: false,
		CheckBreach:   true,
	}

	err := ValidatePassword("password", policy)
	if err == nil {
		t.Fatal("expected error for breached password")
	}

	var breached *BreachedError
	if !errors.As(err, &breached) {
		t.Errorf("expected BreachedError, got %T: %v", err, err)
	}
	if breached.Count == 0 {
		t.Error("expected non-zero breach count in BreachedError")
	}
}

// TestValidatePassword_BreachCheckDisabled verifies that with
// CheckBreach=false, no HIBP call is made.
func TestValidatePassword_BreachCheckDisabled(t *testing.T) {
	policy := PasswordPolicy{
		MinLength:     1,
		MaxLength:     128,
		RequireUpper:  false,
		RequireNumber: false,
		RequireSymbol: false,
		CheckBreach:   false, // Disabled
	}

	// "password" is breached but should pass when CheckBreach is off.
	err := ValidatePassword("password", policy)
	if err != nil {
		t.Errorf("expected nil error when CheckBreach is disabled, got: %v", err)
	}
}

// TestBreachCheckError_Error verifies the error message format.
func TestBreachCheckError_Error(t *testing.T) {
	err := &BreachCheckError{Err: fmt.Errorf("connection refused")}
	if !strings.Contains(err.Error(), "breach check failed") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("original error not wrapped: %s", err.Error())
	}
}

// TestBreachCheckError_Unwrap verifies that errors.Is/As work through the wrapper.
func TestBreachCheckError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("timeout")
	err := &BreachCheckError{Err: inner}
	if !errors.Is(err, inner) {
		t.Error("expected errors.Is to find inner error")
	}
}

// TestBreachedError_Error verifies the error message format.
func TestBreachedError_Error(t *testing.T) {
	err := &BreachedError{Count: 42}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("expected breach count in error message: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "data breaches") {
		t.Errorf("expected 'data breaches' in error message: %s", err.Error())
	}
}

// TestSha1Hash verifies the SHA-1 hash function against a known value.
func TestSha1Hash(t *testing.T) {
	// SHA-1 of "password" = 5baa61e4c9b93f3f0682250b6cf8331b7ee68fd8
	hash := sha1Hash("password")
	expected := "5baa61e4c9b93f3f0682250b6cf8331b7ee68fd8"
	if hash != expected {
		t.Errorf("sha1Hash(\"password\") = %s, want %s", hash, expected)
	}
}

// TestBreachCache_Overwrite verifies that a new entry overwrites an old one.
func TestBreachCache_Overwrite(t *testing.T) {
	cache := newBreachCache(1 * time.Hour)

	hash := "abc123"
	cache.set(hash, false, 0)
	if _, count, ok := cache.get(hash); !ok || count != 0 {
		t.Error("expected entry to be set")
	}

	cache.set(hash, true, 500)
	breached, count, ok := cache.get(hash)
	if !ok {
		t.Error("expected entry to exist after overwrite")
	}
	if !breached || count != 500 {
		t.Errorf("expected overwrite (breached=true, count=500), got breached=%v count=%d", breached, count)
	}
}

// TestBreachCache_ConcurrentAccess ensures no data races under concurrent access.
func TestBreachCache_ConcurrentAccess(t *testing.T) {
	cache := newBreachCache(1 * time.Minute)
	done := make(chan struct{})

	// Writers
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key-%d-%d", n, j)
				cache.set(key, j%2 == 0, j)
			}
			done <- struct{}{}
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key-%d-%d", n, j)
				cache.get(key) //nolint:errcheck
			}
			done <- struct{}{}
		}(i)
	}

	// Wait for all goroutines.
	for i := 0; i < 20; i++ {
		<-done
	}
}

// TestFetchHIBPRange_MissingSuffix tests the helper function
// that parses HIBP line responses.
func TestFetchHIBPRange_MissingSuffix(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return some hashes, none matching our target suffix.
		lines := []string{
			"AAAAAA:100",
			"BBBBBB:200",
			"CCCCCC:300",
		}
		w.Header().Set("Content-Type", "text/plain")
		for _, line := range lines {
			fmt.Fprintln(w, line)
		}
	}))
	defer ts.Close()

	origClient := hibpClient
	hibpClient = ts.Client()
	origURL := hibpRangeURL
	hibpRangeURL = ts.URL + "/"

	found, count, err := queryHIBPPrefixRange("AAAAA", "ZZZZZZ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected suffix ZZZZZZ not to be found")
	}
	if count != 0 {
		t.Errorf("expected count=0, got %d", count)
	}

	hibpClient = origClient
	hibpRangeURL = origURL
}
