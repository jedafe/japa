package valkey

import (
	"context"
	"testing"
	"time"
)

// unreachableAddr is a host:port pair that refuses connections on every
// machine, so the tests never depend on a running Valkey instance.
const unreachableAddr = "localhost:1"

// shortCtx gives the go-redis client a tight deadline for any command, so
// the tests fail fast instead of waiting for the OS-level dial timeout.
func shortCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestNewWithFallback_ReturnsClient(t *testing.T) {
	c := NewWithFallback(unreachableAddr)
	if c == nil {
		t.Fatal("expected non-nil client even with unreachable address")
	}
	if c.Redis() == nil {
		t.Fatal("expected underlying redis client to be non-nil")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestNewWithFallback_PingUnreachable(t *testing.T) {
	c := NewWithFallback(unreachableAddr)
	t.Cleanup(func() { _ = c.Close() })

	err := c.Ping(shortCtx(t))
	if err == nil {
		t.Fatal("expected Ping to fail when the server is unreachable")
	}
}

func TestNew_UnreachableReturnsError(t *testing.T) {
	_, err := New(shortCtx(t), unreachableAddr)
	if err == nil {
		t.Fatal("expected New to return an error when the server is unreachable")
	}
}

func TestClose_DoesNotPanic(t *testing.T) {
	c := NewWithFallback(unreachableAddr)
	// Close should be safe to call exactly once. A panic here would surface
	// as a test failure automatically.
	if err := c.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	c := NewWithFallback(unreachableAddr)
	if err := c.Close(); err != nil {
		t.Fatalf("first Close returned error: %v", err)
	}
	// A second Close may return an error from go-redis but must not panic.
	_ = c.Close()
}

func TestRedis_ReturnsSameInstance(t *testing.T) {
	c := NewWithFallback(unreachableAddr)
	t.Cleanup(func() { _ = c.Close() })

	if c.Redis() != c.Redis() {
		t.Fatal("Redis() should return the same underlying client on every call")
	}
}