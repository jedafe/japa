// intent.go — ADR-128 intent-aware menu shaping. Applied at resolve time in
// GetMenuBySlug so every nav consumer (web shell, mobile shell) inherits it
// without per-client work. This is soft personalization, not access control:
// hidden routes remain reachable by direct URL and RBAC/feature-flag gating
// is unchanged.
//
// Ref: japaconnect-engineering/adr/ADR-128-intent-experience-personalization.md
package menus

import (
	"context"
	"strings"

	"github.com/jedafe/japa/internal/visitor"
)

// applyIntentExperience applies the intent_matrix rule for the request's
// resolved japa_intent: hide_paths entries drop items (subtrees included),
// pin_paths entries promote matches ahead of unpinned siblings. It is a
// no-op when the geo_personalization feature flag is off, the visitor has
// no declared intent, or the matrix carries no rule for it.
func (s *Service) applyIntentExperience(ctx context.Context, items []MenuItem) []MenuItem {
	if s.settings == nil || s.features == nil ||
		!s.features.IsEnabled(ctx, "geo_personalization.enabled") {
		return items
	}

	rule := visitor.IntentRuleFor(ctx, s.settings, visitor.FromContext(ctx).JapaIntent)
	if rule.Empty() {
		return items
	}

	return filterIntentItems(items, rule)
}

// filterIntentItems drops items whose URL matches a hide_paths prefix (the
// whole subtree goes with the parent) and partitions the survivors into
// pinned/unpinned groups so pinned entries sort first while each partition
// keeps its configured relative order.
func filterIntentItems(items []MenuItem, rule visitor.IntentRule) []MenuItem {
	var pinned, unpinned []MenuItem

	for _, it := range items {
		if pathMatchesAny(it.URL, rule.HidePaths) {
			continue
		}

		it.Children = filterIntentItems(it.Children, rule)
		if pathMatchesAny(it.URL, rule.PinPaths) {
			pinned = append(pinned, it)
		} else {
			unpinned = append(unpinned, it)
		}
	}

	return append(pinned, unpinned...)
}

// pathMatchesAny reports whether url matches any prefix segment-wise:
// "/tools" matches "/tools" and "/tools/x" but not "/toolsmith".
func pathMatchesAny(url string, prefixes []string) bool {
	u := strings.TrimSpace(url)

	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		if p != "" && (u == p || strings.HasPrefix(u, p+"/")) {
			return true
		}
	}

	return false
}
