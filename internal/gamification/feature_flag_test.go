package gamification

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jedafe/japa/internal/features"
)

// fakeFeatureChecker returns fixed flag results; unknown keys read disabled,
// matching features.Checker's fail-closed default. evalResults optionally
// override the scoped-eval outcome per key so surface/geo/audience-scoped
// flags can be simulated.
type fakeFeatureChecker struct {
	enabled     map[string]bool
	evalResults map[string]features.EvalResult
}

func (f fakeFeatureChecker) IsEnabled(_ context.Context, key string) bool {
	return f.enabled[key]
}

func (f fakeFeatureChecker) EvalFlag(_ context.Context, key string, _ features.CallerScope) features.EvalResult {
	if res, ok := f.evalResults[key]; ok {
		return res
	}
	return features.EvalResult{Enabled: f.enabled[key], Visible: f.enabled[key]}
}

func strPtr(s string) *string { return &s }

func TestFeatureAvailable(t *testing.T) {
	ctx := context.Background()
	checker := fakeFeatureChecker{enabled: map[string]bool{
		"wiki.enabled":    true,
		"feature.academy": false,
	}}

	tests := []struct {
		name    string
		checker FeatureChecker
		flag    *string
		want    bool
	}{
		{"nil flag is always available", checker, nil, true},
		{"empty flag is always available", checker, strPtr(""), true},
		{"enabled flag is available", checker, strPtr("wiki.enabled"), true},
		{"disabled flag is hidden", checker, strPtr("feature.academy"), false},
		{"unknown flag fails closed", checker, strPtr("feature.nonexistent"), false},
		{"nil checker fails closed on a set flag", nil, strPtr("wiki.enabled"), false},
		{"nil checker with nil flag is available", nil, nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{checker: tc.checker}
			if got := s.featureAvailable(ctx, tc.flag); got != tc.want {
				t.Errorf("featureAvailable(%v) = %v, want %v", tc.flag, got, tc.want)
			}
		})
	}
}

func TestFeatureVisible(t *testing.T) {
	ctx := context.Background()
	sc := features.CallerScope{Surface: "main"}
	checker := fakeFeatureChecker{
		enabled: map[string]bool{"wiki.enabled": true, "feature.academy": false},
		evalResults: map[string]features.EvalResult{
			// Globally on, but scoped away from this caller (e.g. geo or
			// audience rule) — must hide on public surfaces.
			"feature.forum": {Enabled: true, Visible: false},
			"jobs.enabled":  {Enabled: false, Visible: true},
		},
	}

	tests := []struct {
		name    string
		checker FeatureChecker
		flag    *string
		want    bool
	}{
		{"nil flag is always visible", checker, nil, true},
		{"empty flag is always visible", checker, strPtr(""), true},
		{"enabled+visible flag is visible", checker, strPtr("wiki.enabled"), true},
		{"disabled flag is hidden", checker, strPtr("feature.academy"), false},
		{"enabled but not visible hides", checker, strPtr("feature.forum"), false},
		{"visible but not enabled hides", checker, strPtr("jobs.enabled"), false},
		{"unknown flag fails closed", checker, strPtr("feature.nonexistent"), false},
		{"nil checker fails closed on a set flag", nil, strPtr("wiki.enabled"), false},
		{"nil checker with nil flag is visible", nil, nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{checker: tc.checker}
			if got := s.featureVisible(ctx, tc.flag, sc); got != tc.want {
				t.Errorf("featureVisible(%v) = %v, want %v", tc.flag, got, tc.want)
			}
		})
	}
}

// A checker that cannot build scopes leaves CallerScope at its zero value —
// global flag behavior — so featureVisible degrades to the global toggle.
func TestCallerScopeFallback(t *testing.T) {
	s := &Service{checker: fakeFeatureChecker{enabled: map[string]bool{"wiki.enabled": true}}}

	r, err := http.NewRequest(http.MethodGet, "/api/v1/gamification/badges", nil)
	if err != nil {
		t.Fatal(err)
	}

	sc := s.CallerScope(r)
	if sc.Surface != "" || sc.CountryCode != "" || sc.GeoValid || sc.SuperAdmin ||
		sc.GroupSlugs != nil || sc.Permissions != nil || sc.SubjectID != "" {
		t.Errorf("CallerScope with a non-scope-building checker = %+v, want zero scope", sc)
	}
}

func TestNormalizeFeatureFlag(t *testing.T) {
	if got := normalizeFeatureFlag(nil); got != nil {
		t.Errorf("nil flag = %v, want nil", got)
	}
	if got := normalizeFeatureFlag(strPtr("")); got != nil {
		t.Errorf(`"" flag = %v, want nil — empty means unlinked`, *got)
	}
	if got := normalizeFeatureFlag(strPtr("wiki.enabled")); got == nil || *got != "wiki.enabled" {
		t.Errorf("set flag = %v, want wiki.enabled", got)
	}
}

func TestValidateFeatureFlag(t *testing.T) {
	if err := validateFeatureFlag(nil); err != nil {
		t.Fatalf("nil flag should be valid, got %v", err)
	}
	if err := validateFeatureFlag(strPtr("")); err != nil {
		t.Fatalf("empty flag should be valid, got %v", err)
	}
	if err := validateFeatureFlag(strPtr("totally.bogus.flag")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unregistered flag should be ErrInvalidInput, got %v", err)
	}

	if len(features.Registry) == 0 {
		t.Fatal("features.Registry is empty — cannot validate a registered key")
	}
	registered := features.Registry[0].Key
	if err := validateFeatureFlag(strPtr(registered)); err != nil {
		t.Fatalf("registered flag %q should be valid, got %v", registered, err)
	}
}
