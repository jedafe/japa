// Package japastage — intent.go owns the canonical mapping between the
// journey vocabularies that predate and surround ADR-122:
//
//   - japa_intent (ADR-122): relocation | study | work | family | visit |
//     business — the column on user_profiles.
//   - purpose (ADR-064): the `purpose` custom field used by the visitor
//     resolver and journey API — same concept, different enum (it has
//     vacation/tourism instead of visit and no family).
//   - Onboarding primary_goal option IDs (§09) — a superset vocabulary
//     (lifestyle, unsure, …) that needs to collapse onto japa_intent.
//
// Ref: japaconnect-engineering/adr/ADR-128-intent-experience-personalization.md
package japastage

import "strings"

// NormalizeIntent maps a free-form journey vocabulary value onto one of the
// six japa_intent values (ADR-122). Accepts japa_intent values verbatim,
// `purpose` custom-field values (vacation/tourism → visit), onboarding
// primary_goal option IDs (lifestyle → relocation), and common synonyms.
// Returns "" when the value maps to nothing — callers treat "" as
// "no intent declared", never as an error.
func NormalizeIntent(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "relocation", "relocate", "permanent", "permanent_residence", "lifestyle", "live_abroad":
		return "relocation"
	case "study", "education", "student", "university":
		return "study"
	case "work", "employment", "job", "remote_work":
		return "work"
	case "family", "family_reunion", "join_family", "spouse", "dependent":
		return "family"
	case "visit", "vacation", "tourism", "tourist", "holiday", "travel", "trip", "visitor":
		return "visit"
	case "business", "conference", "trade", "entrepreneur", "business_trip", "investment":
		return "business"
	}

	return ""
}

// PurposeForIntent returns the ADR-064 `purpose` custom-field vocabulary
// value for a japa_intent — the inverse of NormalizeIntent restricted to
// the purpose enum. visit maps to "tourism" (the purpose enum predates the
// `visit` class); the remaining intents map to themselves. Returns "" for
// an invalid or empty intent.
func PurposeForIntent(intent string) string {
	switch NormalizeIntent(intent) {
	case "visit":
		return "tourism"
	case "relocation", "study", "work", "family", "business":
		return NormalizeIntent(intent)
	}

	return ""
}
