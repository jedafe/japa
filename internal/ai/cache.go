// Package ai — cache.go implements a semantic cache for AI responses.
//
// The cache stores JSON-serialized Response values in Valkey so identical
// prompts do not trigger repeated LLM calls. Cache hits are returned directly
// and logged to ai_usage_log with provider="cache" and zero cost, making
// the savings visible in the admin AI usage dashboard (Section 20).
//
// Ref: japaconnect-engineering/conventions/valkey-keys-catalog.md
package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// aiResponseCacheKey returns the Valkey key for a cached AI response.
// Format: jc:ai:response:{tenant_id}:{user_id}:{category}:{sha256(payload)}.
// The user_id is included so cached responses stay scoped to the caller.
func aiResponseCacheKey(tenantID, userID pgtype.UUID, category string, payload map[string]any) string {
	// Normalise payload before hashing so equivalent requests share a key.
	payloadJSON, _ := json.Marshal(payload)
	h := sha256.Sum256(payloadJSON)
	hash := hex.EncodeToString(h[:])

	tenantStr := ""
	if tenantID.Valid {
		tenantStr = pgUUIDToString(tenantID)
	}
	userStr := ""
	if userID.Valid {
		userStr = pgUUIDToString(userID)
	}

	return fmt.Sprintf("jc:ai:response:%s:%s:%s:%s", tenantStr, userStr, category, hash)
}

// pgUUIDToString returns the standard UUID string form, or a zero UUID string
// for invalid UUIDs.
func pgUUIDToString(u pgtype.UUID) string {
	if !u.Valid {
		return "00000000-0000-0000-0000-000000000000"
	}
	return u.String()
}

// cacheResponse serialises a Response and writes it to the cache client.
// Errors are logged and ignored — a cache write must never fail a request.
func cacheResponse(ctx context.Context, cache CacheClient, key string, resp Response, ttlSeconds int) {
	if cache == nil {
		return
	}
	data, err := json.Marshal(resp)
	if err != nil {
		slog.Warn("ai: failed to marshal cache response", "error", err)
		return
	}
	if err := cache.Set(ctx, key, string(data), ttlSeconds); err != nil {
		slog.Warn("ai: failed to write response cache", "error", err)
	}
}

// lookupCachedResponse fetches a cached Response. Returns (zero Response, false)
// on cache miss or error.
func lookupCachedResponse(ctx context.Context, cache CacheClient, key string) (Response, bool) {
	if cache == nil {
		return Response{}, false
	}
	data, err := cache.Get(ctx, key)
	if err != nil {
		return Response{}, false
	}
	var resp Response
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		slog.Warn("ai: failed to unmarshal cached response", "error", err)
		return Response{}, false
	}
	return resp, true
}
