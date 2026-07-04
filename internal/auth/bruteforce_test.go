package auth

import (
	"context"
	"testing"
	"time"
)

type mockBruteForceStore struct {
	counts map[string]int
}

func newMockBruteForceStore() *mockBruteForceStore {
	return &mockBruteForceStore{counts: make(map[string]int)}
}

func (m *mockBruteForceStore) GetFailCount(_ context.Context, key string) (int, error) {
	return m.counts[key], nil
}

func (m *mockBruteForceStore) IncrFailCount(_ context.Context, key string) error {
	m.counts[key]++
	return nil
}

func (m *mockBruteForceStore) ResetFailCount(_ context.Context, key string) error {
	delete(m.counts, key)
	return nil
}

func TestCheckLoginAttempt_AllowedUnderThreshold(t *testing.T) {
	t.Parallel()

	store := newMockBruteForceStore()
	ctx := t.Context()

	for i := range MaxLoginAttempts - 1 {
		store.counts["user1"] = i

		allowed, _ := CheckLoginAttempt(ctx, store, "user1")
		if !allowed {
			t.Errorf("attempt %d should be allowed", i+1)
		}
	}
}

func TestCheckLoginAttempt_BlockedAtThreshold(t *testing.T) {
	t.Parallel()

	store := newMockBruteForceStore()
	ctx := t.Context()
	store.counts["user1"] = MaxLoginAttempts

	allowed, retryAfter := CheckLoginAttempt(ctx, store, "user1")
	if allowed {
		t.Error("login should be blocked at threshold")
	}

	if retryAfter <= 0 {
		t.Error("expected positive retry after")
	}
}

func TestRecordAndResetLoginAttempts(t *testing.T) {
	t.Parallel()

	store := newMockBruteForceStore()
	ctx := t.Context()
	id := "user1:ip1"

	// Record failures
	for range 3 {
		if err := RecordFailedLogin(ctx, store, id); err != nil {
			t.Fatalf("RecordFailedLogin failed: %v", err)
		}
	}

	count, _ := store.GetFailCount(ctx, id)
	if count != 3 {
		t.Errorf("expected 3 failures, got %d", count)
	}

	// Reset after successful login
	if err := ResetLoginAttempts(ctx, store, id); err != nil {
		t.Fatalf("ResetLoginAttempts failed: %v", err)
	}

	count, _ = store.GetFailCount(ctx, id)
	if count != 0 {
		t.Errorf("expected 0 after reset, got %d", count)
	}
}

func TestLoginIdentifier(t *testing.T) {
	t.Parallel()

	id := LoginIdentifier("test@example.com", "192.168.1.1")
	if id != "account:test@example.com:ip:192.168.1.1" {
		t.Errorf("unexpected identifier: %s", id)
	}
}

func TestExponentialBackoff(t *testing.T) {
	t.Parallel()

	store := newMockBruteForceStore()
	ctx := t.Context()

	// At 5 failures (threshold): 2^1 = 2 seconds
	store.counts["user1"] = 5

	_, retry := CheckLoginAttempt(ctx, store, "user1")
	if retry != 2*time.Second {
		t.Errorf("at threshold: expected 2s, got %v", retry)
	}

	// At 6 failures: 2^2 = 4 seconds
	store.counts["user1"] = 6

	_, retry = CheckLoginAttempt(ctx, store, "user1")
	if retry != 4*time.Second {
		t.Errorf("at 6: expected 4s, got %v", retry)
	}
}
