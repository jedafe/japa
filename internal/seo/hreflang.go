// Package seo provides SEO utilities including hreflang and sitemap generation.
package seo

import (
	"fmt"
	"strings"
)

// DefaultLanguages lists the site's supported language codes.
var DefaultLanguages = []string{"en", "fr"}

// HreflangLink returns a single HTML <link> tag for a language alternate.
// Example output: <link rel="alternate" hreflang="fr" href="https://example.com/fr/guide" />.
func HreflangLink(lang, href string) string {
	return fmt.Sprintf(`<link rel="alternate" hreflang="%s" href="%s" />`, lang, href)
}

// HreflangAlternate returns all hreflang <link> tags for a page,
// including an x-default pointing to the root-language version.
// baseURL should be the origin (e.g. "https://example.com").
// path should be the page path without a language prefix (e.g. "/guide").
func HreflangAlternate(baseURL, path string, languages []string) string {
	if len(languages) == 0 {
		languages = DefaultLanguages
	}

	var sb strings.Builder

	for _, lang := range languages {
		href := fmt.Sprintf("%s/%s%s", baseURL, lang, path)
		sb.WriteString(HreflangLink(lang, href))
		sb.WriteByte('\n')
	}

	// x-default points to the first language (English root)
	xDefaultHref := fmt.Sprintf("%s/%s%s", baseURL, languages[0], path)
	sb.WriteString(fmt.Sprintf(`<link rel="alternate" hreflang="x-default" href="%s" />`, xDefaultHref))
	sb.WriteByte('\n')

	return sb.String()
}
