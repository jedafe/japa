package bots

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
)

type contextKey struct{ name string }

// BotInfoKey is the context key for detected bot information.
var BotInfoKey = contextKey{"bot_info"}

// BotInfo holds the detected bot information from the middleware.
type BotInfo struct {
	Name   string
	Type   BotType
	Policy BotPolicy
}

const botListCacheTTL = 5 * time.Minute

// BotDetectionMiddleware identifies known bots from the User-Agent header
// and stores bot information in the request context for downstream handlers.
// The middleware queries the bot registry and performs in-memory pattern matching.
// The bot list is cached in Valkey (5-min TTL) to avoid a per-request DB hit.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func BotDetectionMiddleware(store *Store, rdb *valkey.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ua := r.UserAgent()
			if ua == "" {
				next.ServeHTTP(w, r)
				return
			}

			bots, err := getCachedBots(r.Context(), store, rdb)
			if err != nil {
				slog.Warn("bot detection: failed to query bots", "error", err)
				next.ServeHTTP(w, r)

				return
			}

			matched := matchBotsByUserAgent(bots, ua)
			if len(matched) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// Use the first (most specific) match
			bot := matched[0]
			info := &BotInfo{
				Name:   bot.Name,
				Type:   bot.BotType,
				Policy: bot.Policy,
			}

			ctx := context.WithValue(r.Context(), BotInfoKey, info)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// getCachedBots returns the platform's bot list, preferring a Valkey-cached
// copy (jc:bot_list:{}, 5-min TTL) and falling back to the DB.
func getCachedBots(ctx context.Context, store *Store, rdb *valkey.Client) ([]gen.KnownBot, error) {
	cacheKey := valkey.MustKey("bot_list:")

	if rdb != nil {
		cached, err := rdb.Redis().Get(ctx, cacheKey).Result()
		if err == nil && cached != "" {
			var bots []gen.KnownBot
			if err := json.Unmarshal([]byte(cached), &bots); err == nil {
				return bots, nil
			}
		}
	}

	bots, err := store.listKnownBots(ctx, "", MaxPageSize, 0)
	if err != nil {
		return nil, err
	}

	if rdb != nil {
		if data, err := json.Marshal(bots); err == nil {
			if err := rdb.Redis().Set(ctx, cacheKey, data, botListCacheTTL).Err(); err != nil {
				slog.Warn("bot detection: failed to cache bot list", "error", err)
			}
		}
	}

	return bots, nil
}

// BotInfoFromContext extracts the detected bot information from the request
// context. Returns nil if no bot was detected (middleware not applied or
// User-Agent did not match any known bot).
func BotInfoFromContext(ctx context.Context) *BotInfo {
	info, _ := ctx.Value(BotInfoKey).(*BotInfo)
	return info
}

// IsBotRequest checks whether the current request is from a known bot.
func IsBotRequest(ctx context.Context) bool {
	return BotInfoFromContext(ctx) != nil
}

// GetBotPolicy returns the policy for the detected bot, or "allow" if
// no bot was detected.
func GetBotPolicy(ctx context.Context) BotPolicy {
	if info := BotInfoFromContext(ctx); info != nil {
		return info.Policy
	}

	return PolicyAllow
}

// FilterUserAgentsForLogs sanitizes the User-Agent string for logging,
// removing potential injection vectors.
func FilterUserAgentsForLogs(ua string) string {
	if len(ua) > 256 {
		ua = ua[:256]
	}

	return strings.NewReplacer("\n", "", "\r", "", "\t", "").Replace(ua)
}
