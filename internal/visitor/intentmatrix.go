// intent_matrix.go — resolver for the geo_personalization.intent_matrix
// system setting (ADR-128). The matrix maps a japa_intent (or "*") to an
// IntentRule describing how a surface should adapt: which nav paths to
// hide, which to pin, and which space types to demote in discovery. Shared
// here so menus, trending, and onboarding read one contract.
//
// The setting is behavioral configuration, not a feature reachability
// toggle — the geo_personalization.enabled feature flag stays the single
// killswitch and is checked by callers before consulting the matrix.
//
// Ref: japaconnect-engineering/adr/ADR-128-intent-experience-personalization.md
package visitor

import (
	"context"
	"encoding/json"
	"strings"
)

// IntentRule is the per-journey-class experience rule decoded from
// geo_personalization.intent_matrix. All fields are optional; an absent or
// empty rule means "no adaptation".
type IntentRule struct {
	// HidePaths are URL path prefixes removed from resolved menus. Matching
	// is segment-aware — "/tools" hides "/tools/x" but not "/toolsmith".
	// Hiding a parent drops its whole subtree.
	HidePaths []string `json:"hide_paths"`

	// PinPaths are URL path prefixes whose menu entries are moved ahead of
	// unpinned siblings, preserving relative order.
	PinPaths []string `json:"pin_paths"`

	// DemoteSpaceTypes are space_types.slug values ordered last in space
	// discovery (/suggestions/spaces, onboarding recommendations).
	DemoteSpaceTypes []string `json:"demote_space_types"`
}

// Empty reports whether the rule carries no adaptation directives.
func (r IntentRule) Empty() bool {
	return len(r.HidePaths) == 0 && len(r.PinPaths) == 0 && len(r.DemoteSpaceTypes) == 0
}

// StringGetter is the minimal settings-reader contract the matrix resolver
// needs; *config.Settings satisfies it.
type StringGetter interface {
	GetStringOr(ctx context.Context, key, fallback string) string
}

// IntentRuleFor resolves the effective experience rule for a journey class:
// the intent-keyed rule merged over the "*" wildcard rule. Returns the zero
// rule when the intent is empty, the setting is missing or invalid, or the
// reader is nil.
func IntentRuleFor(ctx context.Context, settings StringGetter, intent string) IntentRule {
	if settings == nil || intent == "" {
		return IntentRule{}
	}

	raw := settings.GetStringOr(ctx, "geo_personalization.intent_matrix", "")
	if raw == "" {
		return IntentRule{}
	}

	var matrix map[string]IntentRule
	if err := json.Unmarshal([]byte(raw), &matrix); err != nil {
		return IntentRule{}
	}

	base := matrix["*"]
	rule := matrix[intent]

	return IntentRule{
		HidePaths:        mergeUnique(base.HidePaths, rule.HidePaths),
		PinPaths:         mergeUnique(base.PinPaths, rule.PinPaths),
		DemoteSpaceTypes: mergeUnique(base.DemoteSpaceTypes, rule.DemoteSpaceTypes),
	}
}

// mergeUnique concatenates two string lists, dropping duplicates and empties
// (the second list wins ordering ties).
func mergeUnique(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(a, b...) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}

	return out
}
