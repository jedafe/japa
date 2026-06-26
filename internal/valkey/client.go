// Package valkey provides a thin wrapper around the go-redis client for
// connecting to the Valkey cache and queue service.
//
// Valkey is the BSD-licensed, protocol-compatible fork of Redis used as
// JapaConnect's cache layer and Asynq queue backing. The address of the
// running instance comes from config.Config.RedisAddr (environment variable
// JC_REDIS_ADDR), which defaults to "localhost:6379".
//
// All keys stored in Valkey MUST be namespaced with the "jc:" prefix so
// JapaConnect entries are easy to identify and never collide with any other
// service sharing the same instance. This package does not enforce the prefix
// — callers are responsible for key naming. A common pattern is to nest the
// subsystem under the prefix, for example "jc:cache:...", "jc:ratelimit:...",
// "jc:editlock:...", or "jc:session:...".
//
// The package offers two constructors. New performs an immediate Ping and
// fails if the server is unreachable, so the caller can decide whether to
// proceed. NewWithFallback returns a non-nil Client even when the server is
// down, following the same graceful-degradation pattern used by the GeoIP
// reader in internal/geoip: the server boots, and individual cache operations
// simply fail until the dependency comes back.
package valkey

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// dialTimeout is the maximum time NewWithFallback will wait when establishing
// a connection to an unreachable server. It is short enough that a missing
// Valkey instance does not noticeably slow startup, but long enough that a
// healthy but briefly slow server can still connect.
const dialTimeout = 3 * time.Second

// Client wraps a *redis.Client from go-redis. Other packages should call
// Redis() when they need direct access to the underlying client (for example
// to run pipeline or pub/sub commands); the wrapper methods cover the common
// Ping and Close operations.
type Client struct {
	rdb *redis.Client
}

// New connects to the Valkey server at addr and confirms the connection with
// a Ping. If the connection or Ping fails, the returned error describes the
// failure and the caller decides whether to degrade gracefully or abort.
//
// addr is the host:port of the Valkey instance (for example "localhost:6379").
func New(ctx context.Context, addr string) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		// Close the client so the connection goroutine does not linger.
		_ = rdb.Close()
		return nil, fmt.Errorf("valkey ping %s: %w", addr, err)
	}
	return &Client{rdb: rdb}, nil
}

// NewWithFallback returns a Client configured to talk to addr even when the
// Valkey server is unreachable at construction time. The dial timeout is set
// to 3 seconds so an absent server does not stall startup; subsequent
// commands will fail fast and callers can treat those failures as cache
// misses. This mirrors the GeoIP graceful-degradation model: missing
// dependencies degrade features rather than crashing the server.
func NewWithFallback(addr string) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:        addr,
		DialTimeout: dialTimeout,
	})
	return &Client{rdb: rdb}
}

// Ping verifies that the Valkey server responds. Returns the underlying
// error (for example a dial or context error) when the server is unreachable.
func (c *Client) Ping(ctx context.Context) error {
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("valkey ping: %w", err)
	}
	return nil
}

// Close releases the connection pool. It is safe to call on a client created
// by either New or NewWithFallback, including one whose server was never
// reachable.
func (c *Client) Close() error {
	if err := c.rdb.Close(); err != nil {
		return fmt.Errorf("valkey close: %w", err)
	}
	return nil
}

// Redis returns the underlying *redis.Client for direct access by other
// packages. Use it when the wrapper does not yet expose a required operation
// (pipelines, pub/sub, scripting). Remember to namespace every key with
// "jc:" as described in the package comment.
func (c *Client) Redis() *redis.Client {
	return c.rdb
}