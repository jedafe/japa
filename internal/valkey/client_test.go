package valkey

import (
	"context"
	"testing"
	"time"
)

// unreachableAddr is a host:port pair that refuses connections on every
// machine, so the tests never depend on a running Valkey instance.
const unreachableAddr = "localhost:1"

// shortCtx gives the valkey-go client a tight deadline for any command, so
// the tests fail fast instead of waiting for the OS-level dial timeout.
func shortCtx(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	t.Cleanup(cancel)

	return ctx
}

func TestNewWithFallback_ReturnsClient(t *testing.T) {
	t.Parallel()

	c := NewWithFallback(unreachableAddr)
	if c == nil {
		t.Fatal("expected non-nil client even with unreachable address")
	}

	// In degraded mode, Redis() returns nil — callers check for nil
	// and degrade gracefully (no-ops on cache misses).
	if c.Redis() != nil {
		t.Fatal("expected nil Redis() for unreachable address (degraded mode)")
	}

	c.Close()
}

func TestNewWithFallback_PingUnreachable(t *testing.T) {
	t.Parallel()

	c := NewWithFallback(unreachableAddr)

	t.Cleanup(func() { c.Close() })

	err := c.Ping(shortCtx(t))
	if err == nil {
		t.Fatal("expected Ping to fail when the server is unreachable")
	}
}

func TestNew_UnreachableReturnsError(t *testing.T) {
	t.Parallel()

	_, err := New(shortCtx(t), unreachableAddr)
	if err == nil {
		t.Fatal("expected New to return an error when the server is unreachable")
	}
}

func TestClose_DoesNotPanic(t *testing.T) {
	t.Parallel()

	c := NewWithFallback(unreachableAddr)
	// Close should be safe to call exactly once. A panic here would surface
	// as a test failure automatically.
	c.Close()
}

func TestClose_Idempotent(t *testing.T) {
	t.Parallel()

	c := NewWithFallback(unreachableAddr)
	c.Close()
	// A second Close must not panic.
	c.Close()
}
