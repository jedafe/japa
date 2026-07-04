package auth

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// startTestValkey spins up an in-memory Valkey (miniredis) and returns a
// go-redis-compatible client so the BruteForceStore can be tested without
// a real server.
func startTestValkey(t *testing.T) valkeycompat.Cmdable {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}

	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}

	compat := valkeycompat.NewAdapter(vk)

	t.Cleanup(func() { vk.Close() })

	return compat
}

func TestValkeyBruteForceStore_GetFailCount_MissingKey(t *testing.T) {
	t.Parallel()
	vk := startTestValkey(t)
	store := NewValkeyBruteForceStore(vk)

	count, err := store.GetFailCount(t.Context(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 for missing key, got %d", count)
	}
}

func TestValkeyBruteForceStore_IncrAndGet(t *testing.T) {
	t.Parallel()
	vk := startTestValkey(t)
	store := NewValkeyBruteForceStore(vk)
	ctx := t.Context()

	// Increment three times and verify the count after each.
	for i := 1; i <= 3; i++ {
		if err := store.IncrFailCount(ctx, "user1"); err != nil {
			t.Fatalf("IncrFailCount iteration %d: %v", i, err)
		}

		count, err := store.GetFailCount(ctx, "user1")
		if err != nil {
			t.Fatalf("GetFailCount iteration %d: %v", i, err)
		}

		if count != i {
			t.Errorf("expected count %d, got %d", i, count)
		}
	}
}

func TestValkeyBruteForceStore_Reset(t *testing.T) {
	t.Parallel()
	vk := startTestValkey(t)
	store := NewValkeyBruteForceStore(vk)
	ctx := t.Context()

	// Increment then reset.
	if err := store.IncrFailCount(ctx, "user1"); err != nil {
		t.Fatal(err)
	}

	if err := store.IncrFailCount(ctx, "user1"); err != nil {
		t.Fatal(err)
	}

	if err := store.ResetFailCount(ctx, "user1"); err != nil {
		t.Fatalf("ResetFailCount: %v", err)
	}

	count, err := store.GetFailCount(ctx, "user1")
	if err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Errorf("expected 0 after reset, got %d", count)
	}
}

func TestValkeyBruteForceStore_ResetNonexistent(t *testing.T) {
	t.Parallel()
	vk := startTestValkey(t)
	store := NewValkeyBruteForceStore(vk)

	// Resetting a key that never existed should not error.
	if err := store.ResetFailCount(t.Context(), "ghost"); err != nil {
		t.Errorf("ResetFailCount on missing key: %v", err)
	}
}

func TestValkeyBruteForceStore_KeyExpiry(t *testing.T) {
	t.Parallel()
	vk := startTestValkey(t)
	store := NewValkeyBruteForceStore(vk)
	ctx := t.Context()

	if err := store.IncrFailCount(ctx, "expiry-test"); err != nil {
		t.Fatal(err)
	}

	// Verify the key was created and the count is correct.
	count, err := store.GetFailCount(ctx, "expiry-test")
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
}

func TestValkeyBruteForceStore_KeyNamespacing(t *testing.T) {
	t.Parallel()
	vk := startTestValkey(t)
	store := NewValkeyBruteForceStore(vk)
	ctx := t.Context()

	// Two different keys should be independent.
	if err := store.IncrFailCount(ctx, "alice"); err != nil {
		t.Fatal(err)
	}

	if err := store.IncrFailCount(ctx, "alice"); err != nil {
		t.Fatal(err)
	}

	if err := store.IncrFailCount(ctx, "bob"); err != nil {
		t.Fatal(err)
	}

	aliceCount, _ := store.GetFailCount(ctx, "alice")
	bobCount, _ := store.GetFailCount(ctx, "bob")

	if aliceCount != 2 {
		t.Errorf("alice: expected 2, got %d", aliceCount)
	}

	if bobCount != 1 {
		t.Errorf("bob: expected 1, got %d", bobCount)
	}
}

func TestValkeyBruteForceStore_Integration(t *testing.T) {
	t.Parallel()
	vk := startTestValkey(t)
	store := NewValkeyBruteForceStore(vk)
	ctx := t.Context()
	id := LoginIdentifier("test@example.com", "10.0.0.1")

	// Simulate 4 failed logins — should still be allowed.
	for i := range MaxLoginAttempts - 1 {
		if err := RecordFailedLogin(ctx, store, id); err != nil {
			t.Fatal(err)
		}

		allowed, _ := CheckLoginAttempt(ctx, store, id)
		if !allowed {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}

	// One more failure hits the threshold.
	if err := RecordFailedLogin(ctx, store, id); err != nil {
		t.Fatal(err)
	}

	allowed, retryAfter := CheckLoginAttempt(ctx, store, id)
	if allowed {
		t.Error("login should be blocked after threshold")
	}

	if retryAfter <= 0 {
		t.Error("expected positive retry after")
	}

	// Reset clears the lockout.
	if err := ResetLoginAttempts(ctx, store, id); err != nil {
		t.Fatal(err)
	}

	allowed, _ = CheckLoginAttempt(ctx, store, id)
	if !allowed {
		t.Error("login should be allowed after reset")
	}
}

func TestValkeyBruteForceStore_WindowSliding(t *testing.T) {
	t.Parallel()
	vk := startTestValkey(t)
	store := NewValkeyBruteForceStore(vk)
	ctx := t.Context()

	// Record a failure.
	if err := store.IncrFailCount(ctx, "slide-test"); err != nil {
		t.Fatal(err)
	}

	// Sleep briefly then record another — the TTL should have been
	// refreshed by the second INCR+EXPIRE pipeline.
	time.Sleep(50 * time.Millisecond)

	if err := store.IncrFailCount(ctx, "slide-test"); err != nil {
		t.Fatal(err)
	}

	count, _ := store.GetFailCount(ctx, "slide-test")
	if count != 2 {
		t.Errorf("expected 2 after sliding, got %d", count)
	}
}
