// Package valkey provides a thin wrapper around the valkey-go client for
// connecting to the Valkey cache and queue service.
//
// Valkey is the BSD-licensed, protocol-compatible fork of Redis used as
// JapaConnect's cache layer and Asynq queue backing. The address of the
// running instance comes from config.Config.RedisAddr (environment variable
// JC_REDIS_ADDR), which defaults to "localhost:6379".
//
// All keys stored in Valkey MUST be namespaced with the "jc:" prefix so
// JapaConnect entries are easy to identify and never collide with any other
// service sharing the same instance. Use the MustKey helper to construct
// keys — it enforces the prefix automatically (VALKEY-004). A common pattern
// is to nest the subsystem under the prefix, for example "jc:cache:...",
// "jc:ratelimit:...", "jc:editlock:...", or "jc:session:...".
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
	"strings"

	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// keyPrefix is the mandatory namespace for all JapaConnect Valkey keys.
// Every key stored in the shared Valkey instance MUST begin with this prefix
// so JapaConnect entries are easy to identify and never collide with any
// other service sharing the same instance (Rule 4).
const keyPrefix = "jc:"

// MustKey constructs a Valkey key by prepending the mandatory "jc:" prefix
// and joining the pattern with the given args using ":" as the separator.
// If the pattern already starts with "jc:", the prefix is not doubled.
//
// Example:
//
//	MustKey("docs:page:%s:%s", audience, slug) → "jc:docs:page:public:getting-started"
//
// This helper enforces Rule 4 (VALKEY-004): all keys must be namespaced
// with "jc:". Callers should use this instead of manually constructing keys.
func MustKey(pattern string, args ...any) string {
	key := pattern
	if len(args) > 0 {
		key = fmt.Sprintf(pattern, args...)
	}
	if strings.HasPrefix(key, keyPrefix) {
		return key
	}
	return keyPrefix + key
}

// Client wraps a valkey.Client with a go-redis-compatible adapter.
// Other packages should call Redis() when they need direct access to the
// underlying client (for example to run pipeline or pub/sub commands);
// the wrapper methods cover the common Ping and Close operations.
type Client struct {
	vk  valkey.Client
	rdb valkeycompat.Cmdable
}

// New connects to the Valkey server at addr and confirms the connection with
// a Ping. If the connection or Ping fails, the returned error describes the
// failure and the caller decides whether to degrade gracefully or abort.
//
// addr is the host:port of the Valkey instance (for example "localhost:6379").
func New(ctx context.Context, addr string) (*Client, error) {
	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{addr},
		DisableCache: true,
	})
	if err != nil {
		return nil, fmt.Errorf("valkey connect %s: %w", addr, err)
	}

	rdb := valkeycompat.NewAdapter(vk)

	if err := rdb.Ping(ctx).Err(); err != nil {
		vk.Close()
		return nil, fmt.Errorf("valkey ping %s: %w", addr, err)
	}

	return &Client{vk: vk, rdb: rdb}, nil
}

// NewWithFallback returns a Client configured to talk to addr even when the
// Valkey server is unreachable at construction time. When the server cannot
// be reached, the returned Client operates in a nil-safe degraded mode: all
// operations return zero values and errors, following the same graceful-
// degradation pattern used by the GeoIP reader in internal/geoip.
func NewWithFallback(addr string) *Client {
	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{addr},
		DisableCache: true,
	})
	if err != nil {
		// Server unreachable — return a Client in degraded mode.
		return &Client{}
	}

	rdb := valkeycompat.NewAdapter(vk)

	return &Client{vk: vk, rdb: rdb}
}

// Ping verifies that the Valkey server responds. Returns an error when the
// underlying client is nil (degraded mode) or the server is unreachable.
func (c *Client) Ping(ctx context.Context) error {
	if c.rdb == nil {
		return fmt.Errorf("valkey ping: client not initialised (degraded mode)")
	}

	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("valkey ping: %w", err)
	}

	return nil
}

// Close releases the connection pool. It is safe to call on a client created
// by either New or NewWithFallback, including one whose server was never
// reachable.
func (c *Client) Close() {
	if c.vk != nil {
		c.vk.Close()
	}
}

// Redis returns the go-redis-compatible adapter for direct access by other
// packages. Returns nil when the client is in degraded mode (server
// unreachable). Callers that receive a nil Cmdable should treat all
// operations as no-ops. Use MustKey to construct keys with the mandatory
// "jc:" prefix (VALKEY-004).
func (c *Client) Redis() valkeycompat.Cmdable {
	return c.rdb
}
