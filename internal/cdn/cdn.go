// Package cdn implements the provider-pluggable edge CDN integration
// (ADR-134): publish-time cache purge, provider country headers for the
// geoip fallback path, and cache-tag emission.
//
// The provider is selected by the system_settings key
// "integrations.cdn.provider" ("cloudflare" | "bunnycdn" | empty = disabled)
// and per-provider credentials live in system_settings too, so operators
// switch providers from the Control Panel without a redeploy (Rule 3).
// Wiring happens once at server startup via SetConfigResolver; an empty
// provider disables every behaviour in this package.
package cdn

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Provider identifiers for integrations.cdn.provider.
const (
	ProviderCloudflare = "cloudflare"
	ProviderBunnyCDN   = "bunnycdn"
)

// Config is the resolved CDN configuration for one purge/geo decision.
// Resolved fresh from system_settings on every call so Control Panel edits
// take effect without a restart.
type Config struct {
	// Provider is the integrations.cdn.provider value ("" = disabled).
	Provider string
	// SiteURL is the public site base (JC_SITE_URL) used to build purge URLs.
	SiteURL string

	CloudflareZoneID   string
	CloudflareAPIToken string

	BunnyPullZoneID string
	BunnyAPIKey     string
}

// configResolver returns the current CDN configuration. Set once at server
// startup via SetConfigResolver; nil restores the disabled default. Stored
// atomically so reads stay lock-free and tests can swap it safely (same
// pattern as clientip.trustedPtr).
var configResolver atomic.Pointer[func() Config]

// SetConfigResolver wires the system_settings-backed config source.
func SetConfigResolver(fn func() Config) {
	if fn == nil {
		configResolver.Store(nil)

		return
	}

	configResolver.Store(&fn)
}

// resolve returns the live config, or a disabled zero Config when unwired.
func resolve() Config {
	if fn := configResolver.Load(); fn != nil {
		return (*fn)()
	}

	return Config{}
}

// Enabled reports whether an edge CDN provider is configured.
func Enabled() bool {
	return providerOf(resolve()) != ""
}

// providerOf normalises the configured provider value.
func providerOf(c Config) string {
	switch strings.ToLower(strings.TrimSpace(c.Provider)) {
	case ProviderCloudflare:
		return ProviderCloudflare
	case ProviderBunnyCDN:
		return ProviderBunnyCDN
	default:
		return ""
	}
}

// SiteURL returns the configured public site base URL ("" when disabled).
func SiteURL() string {
	return siteURLFor(resolve())
}

func siteURLFor(c Config) string {
	return strings.TrimRight(c.SiteURL, "/")
}

// CountryHeader returns the request header through which the configured CDN
// reports the visitor's ISO-3166 alpha-2 country code, or "" when the
// provider does not supply one. Cloudflare sends CF-IPCountry; BunnyCDN
// sends CDN-RequestCountryCode.
func CountryHeader() string {
	return countryHeaderFor(providerOf(resolve()))
}

func countryHeaderFor(provider string) string {
	switch provider {
	case ProviderCloudflare:
		return "CF-IPCountry"
	case ProviderBunnyCDN:
		return "CDN-RequestCountryCode"
	default:
		return ""
	}
}

// CacheTagHeader returns the response header the configured CDN reads for
// tag-based purge, or "" when none is configured. Cloudflare reads
// CF-Cache-Tag (purge-by-tag is Enterprise-only); BunnyCDN reads CDN-Tag
// (tag purge works on all plans).
func CacheTagHeader() string {
	return cacheTagHeaderFor(providerOf(resolve()))
}

func cacheTagHeaderFor(provider string) string {
	switch provider {
	case ProviderCloudflare:
		return "CF-Cache-Tag"
	case ProviderBunnyCDN:
		return "CDN-Tag"
	default:
		return ""
	}
}

// SetCacheTags writes the provider's cache-tag header on a response as a
// single comma-separated value — the documented form for both CF-Cache-Tag
// and CDN-Tag. No-op when no CDN is configured or no tags are given.
func SetCacheTags(w http.ResponseWriter, tags ...string) {
	header := CacheTagHeader()
	if header == "" || len(tags) == 0 {
		return
	}

	w.Header().Set(header, strings.Join(tags, ","))
}

// contentRoutePrefix maps content_type to its canonical public detail
// route — the mirror of apps/main/src/lib/utils/contentLink.ts (§14,
// ADR-058). Lives here (not internal/content) so the scheduled-publish
// worker — which cannot import content without an import cycle — can
// build the same URLs.
var contentRoutePrefix = map[string]string{
	"blog_post":       "/blog",
	"guide":           "/guide",
	"discussion":      "/forum",
	"forum_thread":    "/forum", // legacy alias for 'discussion'
	"comparison":      "/comparisons",
	"story":           "/story",
	"achievement":     "/achievements",
	"page":            "/pages",
	"job_listing":     "/jobs",
	"service_listing": "/services",
	"video":           "/video",
	"list":            "/list",
}

// contentDetailURL builds the canonical public detail URL for a content item.
func contentDetailURL(siteBase, contentType, slug string) string {
	prefix, ok := contentRoutePrefix[contentType]
	if !ok {
		prefix = "/content"
	}

	return siteBase + prefix + "/" + slug
}

// siteURLWarned latches so the "no SiteURL" misconfiguration logs once per
// process instead of on every publish.
var siteURLWarned atomic.Bool

// PurgeContentDetail invalidates a content item at the edge: its canonical
// detail page for every slug supplied (pass the old slug too on rename)
// plus the content:{id}/type:{type} tags. Fire-and-forget — see PurgeAsync.
// id is the content UUID as a string. No-op when no CDN is configured.
func PurgeContentDetail(ctx context.Context, id, contentType string, slugs ...string) {
	if !Enabled() || id == "" {
		return
	}

	req := PurgeRequest{
		Tags: []string{"content:" + id},
	}
	if contentType != "" {
		req.Tags = append(req.Tags, "type:"+contentType)
	}

	if base := SiteURL(); base != "" {
		seen := make(map[string]struct{}, len(slugs))

		for _, slug := range slugs {
			if slug == "" {
				continue
			}

			if _, dup := seen[slug]; dup {
				continue
			}

			seen[slug] = struct{}{}
			req.URLs = append(req.URLs, contentDetailURL(base, contentType, slug))
		}
	} else if siteURLWarned.CompareAndSwap(false, true) {
		// Without JC_SITE_URL no URLs can be built — on Cloudflare
		// non-Enterprise (where tag purge is unavailable) this leaves the
		// edge cache with no invalidation at all.
		slog.Warn("cdn: JC_SITE_URL unset — edge purges send tags only; URL purge disabled")
	}

	PurgeAsync(ctx, req)
}

// PurgePublicPaths invalidates site-relative public paths — used by entities
// that are not contents rows (scam alerts, mutual-aid posts, planning groups,
// digest editions, tool pages). paths are URL paths ("/scam-alerts/{id}");
// when JC_SITE_URL is unset only the tags are sent. Fire-and-forget — see
// PurgeAsync.
func PurgePublicPaths(ctx context.Context, tags []string, paths ...string) {
	if !Enabled() {
		return
	}

	req := PurgeRequest{Tags: tags}
	if base := SiteURL(); base != "" {
		for _, p := range paths {
			req.URLs = append(req.URLs, base+p)
		}
	}

	PurgeAsync(ctx, req)
}

// PurgeRequest describes one edge-cache invalidation.
type PurgeRequest struct {
	// Tags are provider cache tags (e.g. "content:{id}", "type:{slug}").
	Tags []string
	// URLs are absolute URLs to purge (e.g. canonical content detail pages).
	URLs []string
}

// Purge invalidates the given tags/URLs on the configured provider.
// No-op (nil) when no provider is configured or the request is empty.
// Provider errors are returned to the caller.
func Purge(ctx context.Context, req PurgeRequest) error {
	cfg := resolve()

	var err error

	switch providerOf(cfg) {
	case ProviderCloudflare:
		err = purgeCloudflare(ctx, cfg, req)
	case ProviderBunnyCDN:
		err = purgeBunny(ctx, cfg, req)
	default:
		return nil
	}

	recordPurge(req, err)

	return err
}

// PurgeAsync fires a purge detached from the request lifecycle
// (Rule 8 — invalidation must never block or fail the publish path).
// Errors are logged, not returned.
func PurgeAsync(ctx context.Context, req PurgeRequest) {
	if !Enabled() || (len(req.Tags) == 0 && len(req.URLs) == 0) {
		return
	}

	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)

	go func() {
		defer cancel()

		if err := Purge(detached, req); err != nil {
			slog.Warn("cdn: edge purge failed", "error", err,
				"urls", len(req.URLs), "tags", len(req.Tags))
		}
	}()
}

// httpClient is shared by all provider calls — short timeout so a hung CDN
// API cannot stall an async purge goroutine.
var httpClient = &http.Client{Timeout: 10 * time.Second}
