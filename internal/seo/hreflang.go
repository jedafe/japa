// Package seo provides SEO utilities including hreflang and sitemap generation.
package seo

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// hreflangSettingsReader is the subset of config.Settings the hreflang helpers
// need to read i18n.enabled_languages and i18n.default_language from
// system_settings. Declared as a local interface so the seo package does not
// import config (avoids an import cycle); *config.Settings satisfies it
// structurally.
// Ref: japaconnect-engineering/sections/26-seo-performance.md
//
//	conventions/system-settings-catalog.md (i18n.enabled_languages)
type hreflangSettingsReader interface {
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string
}

// FallbackLanguages is the spec fallback for i18n.enabled_languages when the
// setting is absent or the settings reader is unavailable. Per the
// system-settings-catalog the default is "en,fr". This is a fallback only —
// callers should pass the resolved language list from system_settings.
var FallbackLanguages = []string{"en", "fr"}

// ResolveLanguages reads i18n.enabled_languages from system_settings for the
// given tenant and returns the parsed language list. Falls back to
// FallbackLanguages when the reader is nil or the setting is empty.
// Use this instead of the old hardcoded DefaultLanguages so the hreflang set
// matches the active language selector (Rule 3 — no hardcoded config).
func ResolveLanguages(ctx context.Context, reader hreflangSettingsReader, tenantID pgtype.UUID) []string {
	if reader == nil {
		return FallbackLanguages
	}

	raw := reader.GetStringOr(ctx, tenantID, "i18n.enabled_languages", "en,fr")
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		if l := strings.TrimSpace(p); l != "" {
			out = append(out, l)
		}
	}

	if len(out) == 0 {
		return FallbackLanguages
	}

	return out
}

// HreflangLink returns a single HTML <link> tag for a language alternate.
// Example output: <link rel="alternate" hreflang="fr" href="https://example.com/fr/guide" />.
func HreflangLink(lang, href string) string {
	return fmt.Sprintf(`<link rel="alternate" hreflang="%s" href="%s" />`, lang, href)
}

// HreflangAlternate returns all hreflang <link> tags for a page,
// including an x-default pointing to the default-language version.
// baseURL should be the origin (e.g. "https://example.com").
// path should be the page path without a language prefix (e.g. "/guide").
// languages should come from ResolveLanguages (system_settings); when empty,
// FallbackLanguages is used as a last resort.
func HreflangAlternate(baseURL, path string, languages []string) string {
	if len(languages) == 0 {
		languages = FallbackLanguages
	}

	var sb strings.Builder

	for _, lang := range languages {
		href := fmt.Sprintf("%s/%s%s", baseURL, lang, path)
		sb.WriteString(HreflangLink(lang, href))
		sb.WriteByte('\n')
	}

	// x-default points to the first language (default language root).
	xDefaultHref := fmt.Sprintf("%s/%s%s", baseURL, languages[0], path)
	sb.WriteString(fmt.Sprintf(`<link rel="alternate" hreflang="x-default" href="%s" />`, xDefaultHref))
	sb.WriteByte('\n')

	return sb.String()
}
