package visitor

import (
	"context"
	"testing"
)

func TestNormalizeIntent(t *testing.T) {
	cases := map[string]string{
		// japa_intent values pass through verbatim.
		"relocation": "relocation",
		"study":      "study",
		"work":       "work",
		"family":     "family",
		"visit":      "visit",
		"business":   "business",
		// purpose custom-field vocabulary.
		"vacation": "visit",
		"tourism":  "visit",
		"holiday":  "visit",
		// onboarding primary_goal vocabulary.
		"lifestyle": "relocation",
		// case/whitespace tolerance.
		"  Business ": "business",
		// unmappable.
		"unsure":  "",
		"":        "",
		"unknown": "",
	}

	for in, want := range cases {
		if got := NormalizeIntent(in); got != want {
			t.Errorf("NormalizeIntent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPurposeForIntent(t *testing.T) {
	cases := map[string]string{
		"visit":      "tourism", // purpose enum predates the visit class
		"relocation": "relocation",
		"study":      "study",
		"work":       "work",
		"family":     "family",
		"business":   "business",
		"":           "",
		"bogus":      "",
	}

	for in, want := range cases {
		if got := PurposeForIntent(in); got != want {
			t.Errorf("PurposeForIntent(%q) = %q, want %q", in, got, want)
		}
	}
}

type fakeGetter struct{ value string }

func (f fakeGetter) GetStringOr(context.Context, string, string) string { return f.value }

func TestIntentRuleFor(t *testing.T) {
	matrix := fakeGetter{`{
		"*":     {"pin_paths": ["/feed"]},
		"visit": {"hide_paths": ["/academy"], "demote_space_types": ["university"]}
	}`}

	rule := IntentRuleFor(t.Context(), matrix, "visit")
	if len(rule.HidePaths) != 1 || rule.HidePaths[0] != "/academy" {
		t.Errorf("expected hide_paths [/academy], got %v", rule.HidePaths)
	}
	// "*" wildcard rule merges under the intent rule.
	if len(rule.PinPaths) != 1 || rule.PinPaths[0] != "/feed" {
		t.Errorf("expected pinned /feed from wildcard, got %v", rule.PinPaths)
	}

	if len(rule.DemoteSpaceTypes) != 1 || rule.DemoteSpaceTypes[0] != "university" {
		t.Errorf("expected demote [university], got %v", rule.DemoteSpaceTypes)
	}

	// No declared intent → no rule.
	if got := IntentRuleFor(t.Context(), matrix, ""); !got.Empty() {
		t.Errorf("expected empty rule for empty intent, got %+v", got)
	}

	// Nil reader / missing setting / invalid JSON → zero rule.
	if got := IntentRuleFor(t.Context(), nil, "visit"); !got.Empty() {
		t.Error("expected empty rule for nil reader")
	}

	if got := IntentRuleFor(t.Context(), fakeGetter{""}, "visit"); !got.Empty() {
		t.Error("expected empty rule for missing setting")
	}

	if got := IntentRuleFor(t.Context(), fakeGetter{"{bad json"}, "visit"); !got.Empty() {
		t.Error("expected empty rule for invalid JSON")
	}
}
