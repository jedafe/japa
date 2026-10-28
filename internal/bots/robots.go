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
	robotsCacheKey = "robots_txt"
	robotsCacheTTL = 24 * time.Hour
)

// defaultDisallowPaths are the utility, auth, and session routes that are
// never useful to crawlers. Values ending in "$" use Google's end-of-match
// anchor, needed only where a bare prefix would over-block public routes
// (/me would match /mentors and /messages; /org would match /organizations;
// /studio would match a future /studios landing page for studio-desktop;
// /save guards future /save-* routes). /oauth collides with nothing public,
// so it stays unanchored. Crawlers that don't understand "$" simply skip
// those lines — never over-block.
var defaultDisallowPaths = []string{
	"/api",
	"/admin",
	"/content",
	"/r$", "/r/",
	"/search",
	"/settings",
	"/messages",
	"/notifications",
	"/bookmarks",
	"/dashboard",
	"/create",
	"/digest",
	"/onboarding",
	"/unsubscribe",
	"/logout",
	"/login",
	"/signup",
	"/forgot-password",
	"/reset-password",
	"/verify-email",
	"/verify-totp",
	"/claim-ownership",
	"/verification-request",
	"/oauth",
	"/me$", "/me/",
	"/org$", "/org/",
	"/studio$", "/studio/",
	"/save$", "/save/",
}

// sanitizeRobotsToken strips CR/LF so an admin-entered pattern (or site name)
// cannot inject additional directives into the generated file.
func sanitizeRobotsToken(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")

	return strings.TrimSpace(s)
}

func writeDisallowPaths(sb *strings.Builder) {
	for _, p := range defaultDisallowPaths {
		sb.WriteString("Disallow: ")
		sb.WriteString(p)
		sb.WriteString("\n")
	}
}

// GenerateRobotsTxt builds a robots.txt directive string from the bot registry.
// Blocked bots get a full disallow, limited bots get a crawl-delay, and
// allowed bots get a permissive directive with optional crawl-delay.
// siteURL is the absolute site origin (e.g. https://japa.to) used for the
// Sitemap directive, which RFC 9309 requires to be an absolute URI.
func GenerateRobotsTxt(bots []KnownBot, siteURL string) string {
	var sb strings.Builder

	sb.WriteString("# JapaConnect robots.txt — generated from the known_bots registry (Section 43).\n\n")

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
		pattern := sanitizeRobotsToken(b.UserAgentPattern)
		if pattern == "" {
			continue
		}

		sb.WriteString("User-agent: ")
		sb.WriteString(pattern)
		sb.WriteString("\nDisallow: /\n\n")
	}

	// Limited bots: crawl-delay + the standard utility-path disallows
	for _, b := range limited {
		pattern := sanitizeRobotsToken(b.UserAgentPattern)
		if pattern == "" {
			continue
		}

		sb.WriteString("User-agent: ")
		sb.WriteString(pattern)
		sb.WriteString("\n")

		if b.CrawlDelay > 0 {
			sb.WriteString("Crawl-delay: ")
			sb.WriteString(strconv.Itoa(b.CrawlDelay))
			sb.WriteString("\n")
		}

		writeDisallowPaths(&sb)
		sb.WriteString("\n")
	}

	// Allowed bots: permissive with optional crawl-delay
	for _, b := range allowed {
		pattern := sanitizeRobotsToken(b.UserAgentPattern)
		if pattern == "" {
			continue
		}

		sb.WriteString("User-agent: ")
		sb.WriteString(pattern)
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
	writeDisallowPaths(&sb)
	sb.WriteString("\n")

	// Sitemap reference (absolute URI per RFC 9309)
	sb.WriteString("Sitemap: ")
	sb.WriteString(strings.TrimRight(siteURL, "/"))
	sb.WriteString("/sitemap.xml\n")

	return sb.String()
}

// fallbackRobotsTxt is served when the bot registry can't be read. It keeps
// the standard utility-path disallows and the sitemap pointer so the file is
// still useful — but cannot honor blocked-bot policies (fail-open, since a
// hard Disallow: / on a transient DB error would deindex the site).
func fallbackRobotsTxt(siteURL string) string {
	var sb strings.Builder
	sb.WriteString("User-agent: *\n")
	writeDisallowPaths(&sb)
	sb.WriteString("\nSitemap: ")
	sb.WriteString(strings.TrimRight(siteURL, "/"))
	sb.WriteString("/sitemap.xml\n")

	return sb.String()
}

// RobotsHandler serves dynamically generated robots.txt. The output is
// cached in Valkey with a 24-hour TTL to avoid regenerating on every request.
// siteURL is the configured site origin for the absolute Sitemap URI; when
// empty it falls back to https://r.Host (same convention as SitemapHandler).
// indexable should be !cfg.BlocksIndexing(): on explicitly non-production
// deployments the handler returns a blanket Disallow: / (never cached) so
// stage/dev environments are never crawlable regardless of the bot registry.
func RobotsHandler(store *Store, rdb *valkey.Client, siteURL string, indexable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !indexable {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=300")
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))

			return
		}

		base := siteURL
		if base == "" {
			base = "https://" + r.Host
		}

		// Try cache first
		cacheKey := valkey.MustKey(robotsCacheKey)
		if rdb != nil {
			cached, err := rdb.Redis().Get(r.Context(), cacheKey).Result()
			if err == nil && cached != "" {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("Cache-Control", "public, max-age=86400")
				_, _ = w.Write([]byte(cached))

				return
			}
		}

		// Generate from bot registry
		bots, err := store.GetAllBots(r.Context())
		if err != nil {
			slog.Error("robots: failed to load bots", "error", err)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=3600")
			_, _ = w.Write([]byte(fallbackRobotsTxt(base)))

			return
		}

		txt := GenerateRobotsTxt(bots, base)

		// Cache in Valkey
		if rdb != nil {
			if err := rdb.Redis().Set(r.Context(), cacheKey, txt, robotsCacheTTL).Err(); err != nil {
				slog.Warn("robots: failed to cache", "error", err)
			}
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write([]byte(txt))
	}
}

// InvalidateRobotsCache removes the cached robots.txt.
// Call this after updating the bot registry.
func InvalidateRobotsCache(ctx context.Context, rdb *valkey.Client) {
	if rdb == nil {
		return
	}

	if err := rdb.Redis().Del(ctx, valkey.MustKey(robotsCacheKey)).Err(); err != nil {
		slog.Warn("robots: failed to invalidate cache", "error", err)
	}
}

// InvalidateLLMsCache removes the cached llms.txt files.
func InvalidateLLMsCache(ctx context.Context, rdb *valkey.Client) {
	if rdb == nil {
		return
	}

	keys := []string{
		valkey.MustKey(llmsCacheKey),
		valkey.MustKey(llmsFullCacheKey),
	}
	for _, key := range keys {
		if err := rdb.Redis().Del(ctx, key).Err(); err != nil {
			slog.Warn("llms: failed to invalidate cache", "key", key, "error", err)
		}
	}

	slog.Info("invalidated llms cache")
}
