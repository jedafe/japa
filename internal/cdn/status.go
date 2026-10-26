package cdn

import (
	"sync/atomic"
	"time"
)

// PurgeResult is a snapshot of the most recent edge purge attempt. Surfaced
// on the Control → Settings → Edge CDN status card so operators can verify
// the integration works without reading server logs.
type PurgeResult struct {
	At    time.Time `json:"at"`
	URLs  int       `json:"urls"`
	Tags  int       `json:"tags"`
	Error string    `json:"error,omitempty"`
}

// lastPurge stores the most recent PurgeResult. atomic.Value keeps reads
// lock-free on the request path.
var lastPurge atomic.Value // stores PurgeResult

// recordPurge stores the outcome of one dispatched purge call.
func recordPurge(req PurgeRequest, err error) {
	r := PurgeResult{At: time.Now().UTC(), URLs: len(req.URLs), Tags: len(req.Tags)}
	if err != nil {
		r.Error = err.Error()
	}

	lastPurge.Store(r)
}

// LastPurge returns the most recent purge outcome; ok is false when no
// purge has run since process start.
func LastPurge() (PurgeResult, bool) {
	if v := lastPurge.Load(); v != nil {
		return v.(PurgeResult), true
	}

	return PurgeResult{}, false
}

// Status describes the resolved CDN configuration for the Control Panel
// status card. Credential fields report presence only — values are never
// exposed.
type Status struct {
	// Enabled is true when a recognised provider is configured.
	Enabled bool `json:"enabled"`
	// Provider is the normalised provider id ("" when disabled).
	Provider string `json:"provider"`
	// SiteURL is the base used to build purge URLs — empty means URL
	// purges are silently skipped (tags only).
	SiteURL string `json:"site_url"`
	// CountryHeader / CacheTagHeader are the request/response headers the
	// configured provider uses for geo fallback and tag-based purge.
	CountryHeader  string `json:"country_header,omitempty"`
	CacheTagHeader string `json:"cache_tag_header,omitempty"`
	// CredentialsConfigured is true when the provider's required
	// credentials are present (CF: zone+token; Bunny: api_key).
	CredentialsConfigured bool `json:"credentials_configured"`
	// TagPurgeReady reports whether tag-based purge is expected to work
	// right now — false when provider credentials are absent, when Bunny
	// lacks pull_zone_id, or when Cloudflare rejected a tag purge and the
	// retry cooldown is still running.
	TagPurgeReady bool `json:"tag_purge_ready"`
	// LastPurge is the most recent purge outcome, or absent before the
	// first purge.
	LastPurge *PurgeResult `json:"last_purge,omitempty"`
}

// StatusSnapshot resolves the live configuration into a Status for the
// Control Panel. No secrets are returned — only presence flags.
func StatusSnapshot() Status {
	cfg := resolve()
	provider := providerOf(cfg)

	s := Status{
		Enabled:        provider != "",
		Provider:       provider,
		SiteURL:        siteURLFor(cfg),
		CountryHeader:  countryHeaderFor(provider),
		CacheTagHeader: cacheTagHeaderFor(provider),
	}

	switch provider {
	case ProviderCloudflare:
		s.CredentialsConfigured = cfg.CloudflareZoneID != "" && cfg.CloudflareAPIToken != ""
		s.TagPurgeReady = s.CredentialsConfigured && !cfTagSuppressed()
	case ProviderBunnyCDN:
		s.CredentialsConfigured = cfg.BunnyAPIKey != ""
		s.TagPurgeReady = s.CredentialsConfigured && cfg.BunnyPullZoneID != ""
	}

	if r, ok := LastPurge(); ok {
		s.LastPurge = &r
	}

	return s
}
