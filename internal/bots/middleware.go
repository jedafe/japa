package bots

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jedafe/japa/internal/tenant"
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

// BotDetectionMiddleware identifies known bots from the User-Agent header
// and stores bot information in the request context for downstream handlers.
// The middleware queries the bot registry for the current tenant and performs
// in-memory pattern matching.
func BotDetectionMiddleware(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := tenant.FromContext(r.Context())
			if t == nil {
				next.ServeHTTP(w, r)
				return
			}

			ua := r.UserAgent()
			if ua == "" {
				next.ServeHTTP(w, r)
				return
			}

			bots, err := store.GetBotsByUserAgent(r.Context(), t.ID, ua)
			if err != nil {
				slog.Warn("bot detection: failed to query bots",
					"tenant_id", t.ID,
					"error", err,
				)
				next.ServeHTTP(w, r)
				return
			}

			if len(bots) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// Use the first (most specific) match
			bot := bots[0]
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
