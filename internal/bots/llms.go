package bots

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/valkey"
)

const (
	llmsCacheKey     = "jc:llms_txt"
	llmsFullCacheKey = "jc:llms_full_txt"
	llmsCacheTTL     = 1 * time.Hour
)

// GenerateLLMsTxt produces a brief LLMS.txt file with the site name,
// description, and top-level links. This is the short version served at /llms.txt.
func GenerateLLMsTxt(siteName, siteDescription string, topLinks []Link) string {
	var sb strings.Builder

	sb.WriteString("# ")
	sb.WriteString(siteName)
	sb.WriteString("\n\n")
	sb.WriteString("> ")
	sb.WriteString(siteDescription)
	sb.WriteString("\n\n")

	if len(topLinks) > 0 {
		sb.WriteString("## Top Links\n\n")
		for _, link := range topLinks {
			sb.WriteString("- [")
			sb.WriteString(link.Title)
			sb.WriteString("](")
			sb.WriteString(link.URL)
			sb.WriteString(")\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// GenerateLLMsFullTxt produces a detailed LLMS-full.txt with recent posts,
// tags, and spaces. This is the extended version served at /llms-full.txt.
func GenerateLLMsFullTxt(siteName, siteDescription string, data LLMsData) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# ")
	sb.WriteString(siteName)
	sb.WriteString("\n\n")
	sb.WriteString("> ")
	sb.WriteString(siteDescription)
	sb.WriteString("\n\n")

	// Recent Posts
	if len(data.RecentPosts) > 0 {
		sb.WriteString("## Recent Posts\n\n")
		for _, p := range data.RecentPosts {
			sb.WriteString("- [")
			sb.WriteString(p.Title)
			sb.WriteString("](")
			sb.WriteString(p.URL)
			sb.WriteString(")\n")
		}
		sb.WriteString("\n")
	}

	// Tags
	if len(data.Tags) > 0 {
		sb.WriteString("## Topics\n\n")
		for _, t := range data.Tags {
			sb.WriteString("- [")
			sb.WriteString(t.Title)
			sb.WriteString("](")
			sb.WriteString(t.URL)
			sb.WriteString(")\n")
		}
		sb.WriteString("\n")
	}

	// Spaces
	if len(data.Spaces) > 0 {
		sb.WriteString("## Spaces\n\n")
		for _, s := range data.Spaces {
			sb.WriteString("- [")
			sb.WriteString(s.Title)
			sb.WriteString("](")
			sb.WriteString(s.URL)
			sb.WriteString(")\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// LLMsHandler serves /llms.txt and /llms-full.txt. The output is cached
// in Valkey with a 1-hour TTL. In production, the data source would query
// recent posts, tags, and spaces from the database. For now, the handler
// returns a basic template that can be enhanced with real data.
func LLMsHandler(store *Store, rdb *valkey.Client, siteName, siteDescription string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := tenant.FromContext(r.Context())
		if t == nil {
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Cache-Control", "public, max-age=3600")
			_, _ = w.Write([]byte("# Site\n\n> Site description\n"))
			return
		}

		isFull := strings.HasSuffix(r.URL.Path, "/llms-full.txt")
		cacheKey := llmsCacheKey + ":" + t.ID.String()
		if isFull {
			cacheKey = llmsFullCacheKey + ":" + t.ID.String()
		}

		// Try cache first
		if rdb != nil {
			cached, err := rdb.Redis().Get(r.Context(), cacheKey).Result()
			if err == nil && cached != "" {
				w.Header().Set("Content-Type", "text/plain")
				w.Header().Set("Cache-Control", "public, max-age=3600")
				_, _ = w.Write([]byte(cached))
				return
			}
		}

		var txt string
		if isFull {
			txt = GenerateLLMsFullTxt(siteName, siteDescription, LLMsData{})
		} else {
			txt = GenerateLLMsTxt(siteName, siteDescription, nil)
		}

		// Cache in Valkey
		if rdb != nil {
			if err := rdb.Redis().Set(r.Context(), cacheKey, txt, llmsCacheTTL).Err(); err != nil {
				slog.Warn("llms: failed to cache", "tenant_id", t.ID, "error", err)
			}
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write([]byte(txt))
	}
}

// RegenerateLLMsTxt forces regeneration of the cached LLMS.txt for a tenant.
// Called by the Asynq maintenance task. The tenantID is passed as a string
// to avoid importing pgtype in this file.
func RegenerateLLMsTxt(ctx context.Context, rdb *valkey.Client, tenantID string) error {
	if rdb == nil {
		return nil
	}
	// Delete both cache keys to force regeneration on next request
	keys := []string{
		llmsCacheKey + ":" + tenantID,
		llmsFullCacheKey + ":" + tenantID,
	}
	for _, key := range keys {
		if err := rdb.Redis().Del(ctx, key).Err(); err != nil {
			slog.Warn("llms: failed to invalidate cache", "key", key, "error", err)
		}
	}
	slog.Info("llms: regenerated cache", "tenant_id", tenantID)
	return nil
}
