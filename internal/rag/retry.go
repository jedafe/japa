package rag

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"
)

var RetryDelays = []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}

func sleepContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}

	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func IndexWithRetry(ctx context.Context, entityType, entityID, operation string, fn func(context.Context) error) error {
	delays := RetryDelays
	if len(delays) == 0 {
		delays = []time.Duration{0}
	}

	var err error

	for attempt := 0; attempt <= len(delays); attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err = fn(ctx); err == nil {
			return nil
		}

		if attempt == len(delays) {
			break
		}

		slog.Warn("rag_index_failed",
			"entity_type", entityType,
			"entity_id", entityID,
			"operation", operation,
			"attempt", attempt+1,
			"error", err)

		wait := delays[attempt] + time.Duration(rand.Int64N(int64(delays[attempt]/2+1))) // #nosec G404 -- retry jitter is not security-sensitive
		if !sleepContext(ctx, wait) {
			return err
		}
	}

	slog.Warn("rag_index_failed",
		"entity_type", entityType,
		"entity_id", entityID,
		"operation", operation,
		"attempt", len(delays)+1,
		"error", err)

	return err
}
