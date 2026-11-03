package bots

import (
	"encoding/json"
	"strings"
)

// TipTapText extracts plain text from a TipTap/ProseMirror JSON document for
// word counting. Shared by every indexability gate that measures prose length
// — the wiki page noindex contract (wiki.Service) and the wiki sitemap quality
// floor (seo.ServeWikiSitemap) must count words identically or they drift.
// A bare JSON string (plain-text revision fallback) is used as-is.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func TipTapText(raw []byte) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	var sb strings.Builder
	var walk func(n any)
	walk = func(n any) {
		m, ok := n.(map[string]any)
		if !ok {
			return
		}
		if t, ok := m["text"].(string); ok {
			sb.WriteString(t)
			sb.WriteByte(' ')
		}
		if kids, ok := m["content"].([]any); ok {
			for _, k := range kids {
				walk(k)
			}
		}
	}
	walk(v)
	return sb.String()
}
