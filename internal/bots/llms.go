package bots

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/valkey"
)

const (
	llmsCacheKey     = "llms_txt"
	llmsFullCacheKey = "llms_full_txt"
	llmsCacheTTL     = 1 * time.Hour
)

// sanitizeLLMsTitle strips characters that would corrupt a markdown link or
// inject extra lines into the generated file ([ ] break link text, CR/LF
// break the line format), then collapses whitespace.
func sanitizeLLMsTitle(s string) string {
	r := strings.NewReplacer("\r", " ", "\n", " ", "[", "(", "]", ")")

	return strings.Join(strings.Fields(r.Replace(s)), " ")
}

func writeLLMsHeader(sb *strings.Builder, siteName, siteDescription string) {
	sb.WriteString("# ")
	sb.WriteString(sanitizeLLMsTitle(siteName))
	sb.WriteString("\n\n")
	sb.WriteString("> ")
	sb.WriteString(sanitizeLLMsTitle(siteDescription))
	sb.WriteString("\n\n")
}

func writeLinkSection(sb *strings.Builder, heading string, links []Link) {
	if len(links) == 0 {
		return
	}

	sb.WriteString("## ")
	sb.WriteString(heading)
	sb.WriteString("\n\n")

	for _, link := range links {
		sb.WriteString("- [")
		sb.WriteString(sanitizeLLMsTitle(link.Title))
		sb.WriteString("](")
		sb.WriteString(link.URL)
		sb.WriteString(")")

		if d := sanitizeLLMsTitle(link.Description); d != "" {
			sb.WriteString(": ")
			sb.WriteString(d)
		}

		sb.WriteString("\n")
	}

	sb.WriteString("\n")
}

// writeFeaturedSection inlines article bodies — llms-full.txt's substance,
// per the convention that the "full" file carries content, not just links.
func writeFeaturedSection(sb *strings.Builder, heading string, items []FeaturedItem) {
	if len(items) == 0 {
		return
	}

	sb.WriteString("## ")
	sb.WriteString(heading)
	sb.WriteString("\n\n")

	for _, it := range items {
		sb.WriteString("### [")
		sb.WriteString(sanitizeLLMsTitle(it.Title))
		sb.WriteString("](")
		sb.WriteString(it.URL)
		sb.WriteString(")\n\n")
		sb.WriteString(it.Body)
		sb.WriteString("\n\n")
	}
}

// GenerateLLMsTxt produces the brief /llms.txt: header, curated top-level
// links, recent posts, and an Optional section pointing at llms-full.txt,
// the RSS feed, and the sitemap.
func GenerateLLMsTxt(siteName, siteDescription string, data LLMsBriefData) string {
	var sb strings.Builder

	writeLLMsHeader(&sb, siteName, siteDescription)
	writeLinkSection(&sb, "Top Links", data.TopLinks)
	writeLinkSection(&sb, "Recent Posts", data.RecentPosts)
	writeLinkSection(&sb, "Optional", data.Optional)

	return sb.String()
}

// GenerateLLMsFullTxt produces the extended /llms-full.txt with recent posts,
// topics, spaces, and an Optional section for feeds/discovery endpoints.
func GenerateLLMsFullTxt(siteName, siteDescription string, data LLMsData) string {
	var sb strings.Builder

	writeLLMsHeader(&sb, siteName, siteDescription)
	writeLinkSection(&sb, "Recent Posts", data.RecentPosts)
	writeLinkSection(&sb, "Topics", data.Tags)
	writeLinkSection(&sb, "Spaces", data.Spaces)
	writeFeaturedSection(&sb, "Featured Content", data.Featured)
	writeLinkSection(&sb, "Optional", data.Optional)

	return sb.String()
}

// absolutizeLinks prefixes root-relative URLs with the site origin — llms.txt
// convention (and most consumers) expect absolute markdown links.
func absolutizeLinks(links []Link, base string) []Link {
	out := make([]Link, len(links))
	for i, l := range links {
		if strings.HasPrefix(l.URL, "/") {
			l.URL = base + l.URL
		}
		out[i] = l
	}

	return out
}

// topNavLinks is the curated set of canonical entry points for the brief file.
func topNavLinks(base string) []Link {
	return []Link{
		{Title: "Blog", URL: base + "/blog"},
		{Title: "Forum", URL: base + "/forum"},
		{Title: "Spaces", URL: base + "/spaces"},
		{Title: "Tags", URL: base + "/tags"},
		{Title: "Deals", URL: base + "/deals"},
		{Title: "Tools", URL: base + "/tools"},
		{Title: "Roadmap", URL: base + "/roadmap"},
		{Title: "Academy", URL: base + "/academy"},
		{Title: "Docs", URL: base + "/docs"},
	}
}

// discoveryLinks returns the Optional section: machine-readable discovery
// endpoints. includeFull adds the cross-link to /llms-full.txt (the brief
// file is how crawlers are expected to find it).
func discoveryLinks(base string, includeFull bool) []Link {
	links := make([]Link, 0, 3)
	if includeFull {
		links = append(links, Link{Title: "Extended content index", URL: base + "/llms-full.txt"})
	}

	return append(links,
		Link{Title: "RSS feed", URL: base + "/feed.xml"},
		Link{Title: "Sitemap", URL: base + "/sitemap.xml"},
	)
}

// LLMsHandler serves /llms.txt and /llms-full.txt. The output is cached
// in Valkey with a 1-hour TTL. The brief version carries curated top-level
// links plus the 10 most recent posts; the full version lists recent posts,
// top tags, and active public spaces. Content and spaces pass the same
// seo.sitemap.* quality gates as the sitemaps (settings may be nil → spec
// defaults). On non-indexable deployments the routes 404 — there is
// nothing to crawl.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func LLMsHandler(store *Store, rdb *valkey.Client, settings intSettingsReader, siteName, siteDescription, siteURL string, indexable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !indexable {
			http.NotFound(w, r)

			return
		}

		isFull := strings.HasSuffix(r.URL.Path, "/llms-full.txt")
		cacheKey := valkey.MustKey(llmsCacheKey)

		if isFull {
			cacheKey = valkey.MustKey(llmsFullCacheKey)
		}

		// Try cache first
		if rdb != nil {
			cached, err := rdb.Redis().Get(r.Context(), cacheKey).Result()
			if err == nil && cached != "" {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("Cache-Control", "public, max-age=3600")
				_, _ = w.Write([]byte(cached))

				return
			}
		}

		base := strings.TrimRight(siteURL, "/")
		if base == "" {
			base = "https://" + r.Host
		}

		// Fetch data; on error the nav/discovery links still render (they
		// need no DB), so the file degrades instead of disappearing.
		cacheControl := "public, max-age=3600"
		data, err := store.GetLLMsData(r.Context(), LoadSitemapQualityConfig(r.Context(), settings))
		if err != nil {
			slog.Error("llms: failed to load data", "error", err)
			data = LLMsData{}
			cacheControl = "public, max-age=300"
		}

		var txt string
		if isFull {
			data.RecentPosts = absolutizeLinks(data.RecentPosts, base)
			data.Tags = absolutizeLinks(data.Tags, base)
			data.Spaces = absolutizeLinks(data.Spaces, base)
			for i := range data.Featured {
				if strings.HasPrefix(data.Featured[i].URL, "/") {
					data.Featured[i].URL = base + data.Featured[i].URL
				}
			}
			data.Optional = discoveryLinks(base, false)
			txt = GenerateLLMsFullTxt(siteName, siteDescription, data)
		} else {
			recent := data.RecentPosts
			if len(recent) > 10 {
				recent = recent[:10]
			}

			txt = GenerateLLMsTxt(siteName, siteDescription, LLMsBriefData{
				TopLinks:    topNavLinks(base),
				RecentPosts: absolutizeLinks(recent, base),
				Optional:    discoveryLinks(base, true),
			})
		}

		// Cache in Valkey (never cache the degraded output)
		if err == nil && rdb != nil {
			if err := rdb.Redis().Set(r.Context(), cacheKey, txt, llmsCacheTTL).Err(); err != nil {
				slog.Warn("llms: failed to cache", "error", err)
			}
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", cacheControl)
		_, _ = w.Write([]byte(txt))
	}
}
