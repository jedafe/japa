package rag

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func withTestDelays(t *testing.T, delays []time.Duration) {
	t.Helper()

	old := RetryDelays
	RetryDelays = delays

	t.Cleanup(func() { RetryDelays = old })
}

func TestIndexWithRetry_SucceedsFirstTry(t *testing.T) {
	withTestDelays(t, []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond})

	var calls atomic.Int32

	err := IndexWithRetry(t.Context(), "space", "id-1", "index", func(context.Context) error {
		calls.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("IndexWithRetry = %v, want nil", err)
	}

	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

func TestIndexWithRetry_RetriesThenSucceeds(t *testing.T) {
	withTestDelays(t, []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond})

	var calls atomic.Int32

	err := IndexWithRetry(t.Context(), "wiki_page", "id-2", "index", func(context.Context) error {
		if calls.Add(1) < 3 {
			return errors.New("transient embedder failure")
		}

		return nil
	})
	if err != nil {
		t.Fatalf("IndexWithRetry = %v, want nil", err)
	}

	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}

func TestIndexWithRetry_Exhausts(t *testing.T) {
	withTestDelays(t, []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond})

	var calls atomic.Int32

	sentinel := errors.New("persistent db failure")
	err := IndexWithRetry(t.Context(), "space", "id-3", "purge", func(context.Context) error {
		calls.Add(1)
		return sentinel
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("IndexWithRetry = %v, want sentinel", err)
	}

	if calls.Load() != 4 {
		t.Errorf("calls = %d, want 4 (1 initial + 3 retries)", calls.Load())
	}
}

func TestIndexWithRetry_CancelledContext(t *testing.T) {
	withTestDelays(t, []time.Duration{time.Hour, time.Hour, time.Hour})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var calls atomic.Int32

	err := IndexWithRetry(ctx, "space", "id-4", "index", func(context.Context) error {
		calls.Add(1)
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("IndexWithRetry = nil, want context error")
	}

	if calls.Load() != 0 {
		t.Errorf("calls = %d, want 0 (cancelled before first attempt)", calls.Load())
	}
}
