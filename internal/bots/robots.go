package bots

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/valkey"
)

const (
	robotsCacheKey = "jc:robots_txt"
	robotsCacheTTL = 24 * time.Hour
)

// GenerateRobotsTxt builds a robots.txt directive string from the bot registry.
// Blocked bots get a full disallow, limited bots get a crawl-delay, and
// allowed bots get a permissive directive with optional crawl-delay.
func GenerateRobotsTxt(bots []KnownBot) string {
	var sb strings.Builder

	// Group bots by policy
	var blocked, limited, allowed []KnownBot

	for _, b := range bots {
		switch b.Policy {
		case PolicyBlock:
			blocked = append(blocked, b)
		case PolicyLimit:
			limited = append(limited, b)
		default:
			allowed = append(allowed, b)
		}
	}

	// Blocked bots: disallow everything
	for _, b := range blocked {
		sb.WriteString("User-agent: ")
		sb.WriteString(b.UserAgentPattern)
		sb.WriteString("\nDisallow: /\n\n")
	}

	// Limited bots: crawl-delay + partial disallow
	for _, b := range limited {
		sb.WriteString("User-agent: ")
		sb.WriteString(b.UserAgentPattern)
		sb.WriteString("\n")

		if b.CrawlDelay > 0 {
			sb.WriteString("Crawl-delay: ")
			sb.WriteString(strconv.Itoa(b.CrawlDelay))
			sb.WriteString("\n")
		}

		sb.WriteString("Disallow: /admin/\n")
		sb.WriteString("Disallow: /api/\n\n")
	}

	// Allowed bots: permissive with optional crawl-delay
	for _, b := range allowed {
		sb.WriteString("User-agent: ")
		sb.WriteString(b.UserAgentPattern)
		sb.WriteString("\n")

		if b.CrawlDelay > 0 {
			sb.WriteString("Crawl-delay: ")
			sb.WriteString(strconv.Itoa(b.CrawlDelay))
			sb.WriteString("\n")
		}

		sb.WriteString("Allow: /\n\n")
	}

	// Default policy for unmatched bots
	sb.WriteString("User-agent: *\n")
	sb.WriteString("Disallow: /admin/\n")
	sb.WriteString("Disallow: /api/\n\n")

	// Sitemap reference
	sb.WriteString("Sitemap: /sitemap.xml\n")

	return sb.String()
}

// RobotsHandler serves dynamically generated robots.txt. The output is
// cached in Valkey with a 24-hour TTL to avoid regenerating on every request.
func RobotsHandler(store *Store, rdb *valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Try cache first
		cacheKey := robotsCacheKey
		if rdb != nil {
			cached, err := rdb.Redis().Get(r.Context(), cacheKey).Result()
			if err == nil && cached != "" {
				w.Header().Set("Content-Type", "text/plain")
				w.Header().Set("Cache-Control", "public, max-age=86400")
				_, _ = w.Write([]byte(cached))

				return
			}
		}

		// Generate from bot registry
		bots, err := store.GetAllBots(r.Context())
		if err != nil {
			slog.Error("robots: failed to load bots", "error", err)
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Cache-Control", "public, max-age=3600")
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))

			return
		}

		txt := GenerateRobotsTxt(bots)

		// Cache in Valkey
		if rdb != nil {
			if err := rdb.Redis().Set(r.Context(), cacheKey, txt, robotsCacheTTL).Err(); err != nil {
				slog.Warn("robots: failed to cache", "error", err)
			}
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write([]byte(txt))
	}
}

// InvalidateRobotsCache removes the cached robots.txt for a platform.
// Call this after updating the bot registry.
func InvalidateRobotsCache(ctx context.Context, rdb *valkey.Client) {
	if rdb == nil {
		return
	}

	cacheKey := robotsCacheKey + ":" + ""
	if err := rdb.Redis().Del(ctx, cacheKey).Err(); err != nil {
		slog.Warn("robots: failed to invalidate cache", "error", err)
	}
}

// InvalidateLLMsCache removes the cached llms.txt for a platform.
func InvalidateLLMsCache(ctx context.Context, rdb *valkey.Client) {
	if rdb == nil {
		return
	}

	keys := []string{
		llmsCacheKey + ":" + "",
		llmsFullCacheKey + ":" + "",
	}
	for _, key := range keys {
		if err := rdb.Redis().Del(ctx, key).Err(); err != nil {
			slog.Warn("llms: failed to invalidate cache", "key", key, "error", err)
		}
	}

	slog.Info("invalidated llms cache")
}
