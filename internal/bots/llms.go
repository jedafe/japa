package bots

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
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
// in Valkey with a 1-hour TTL. The brief version includes the site name,
// description, and top-level links (recent published content). The full
// version adds recent posts, top tags, and active public spaces.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func LLMsHandler(store *Store, rdb *valkey.Client, siteName, siteDescription string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isFull := strings.HasSuffix(r.URL.Path, "/llms-full.txt")
		cacheKey := llmsCacheKey

		if isFull {
			cacheKey = llmsFullCacheKey
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

		// Fetch real data from the database
		data, err := store.GetLLMsData(r.Context(), pgtype.UUID{})
		if err != nil {
			slog.Error("llms: failed to load data", "error", err)
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Cache-Control", "public, max-age=300")
			_, _ = w.Write([]byte(GenerateLLMsTxt(siteName, siteDescription, nil)))

			return
		}

		var txt string
		if isFull {
			txt = GenerateLLMsFullTxt(siteName, siteDescription, data)
		} else {
			// Brief version: use recent posts as top-level links
			topLinks := data.RecentPosts
			if len(topLinks) > 10 {
				topLinks = topLinks[:10]
			}

			txt = GenerateLLMsTxt(siteName, siteDescription, topLinks)
		}

		// Cache in Valkey
		if rdb != nil {
			if err := rdb.Redis().Set(r.Context(), cacheKey, txt, llmsCacheTTL).Err(); err != nil {
				slog.Warn("llms: failed to cache", "error", err)
			}
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write([]byte(txt))
	}
}
