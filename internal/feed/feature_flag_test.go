package feed

import (
	"context"
	"testing"

	"github.com/jedafe/japa/internal/features"
)

type stubFeedChecker struct {
	enabled bool
	eval    features.EvalResult
}

func (s stubFeedChecker) IsEnabled(context.Context, string) bool { return s.enabled }
func (s stubFeedChecker) EvalFlag(context.Context, string, features.CallerScope) features.EvalResult {
	return s.eval
}

func TestFeatureVisibleScopeSemantics(t *testing.T) {
	scoped := features.WithScope(context.Background(), features.CallerScope{Surface: "main"})

	tests := []struct {
		name    string
		checker stubFeedChecker
		ctx     context.Context
		want    bool
	}{
		{"scoped: enabled+visible → shown", stubFeedChecker{enabled: true, eval: features.EvalResult{Enabled: true, Visible: true}}, scoped, true},
		{"scoped: enabled+hidden → pruned (projection semantics)", stubFeedChecker{enabled: true, eval: features.EvalResult{Enabled: true, Visible: false}}, scoped, false},
		{"scoped: surface-disabled → pruned", stubFeedChecker{enabled: true, eval: features.EvalResult{Enabled: false, Visible: false}}, scoped, false},
		{"unattached: global enabled → shown", stubFeedChecker{enabled: true, eval: features.EvalResult{Enabled: false}}, context.Background(), true},
		{"unattached: global disabled → pruned", stubFeedChecker{enabled: false, eval: features.EvalResult{Enabled: true, Visible: true}}, context.Background(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{checker: tt.checker}
			if got := s.featureVisible(tt.ctx, "story.enabled"); got != tt.want {
				t.Fatalf("featureVisible = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterDisabledFeatureItemsUsesScope(t *testing.T) {
	s := &Service{checker: stubFeedChecker{
		enabled: true, // globally on
		eval:    features.EvalResult{Enabled: false, Visible: false},
	}}
	items := []FeedItem{
		{ContentType: "story"},
		{ContentType: "guide"}, // ungated — never pruned
	}

	// Attached scope denies → story pruned.
	scoped := features.WithScope(context.Background(), features.CallerScope{Surface: "tools"})
	got := s.filterDisabledFeatureItems(scoped, items)
	if len(got) != 1 || got[0].ContentType != "guide" {
		t.Fatalf("scoped: expected only guide, got %+v", got)
	}

	// Unattached ctx → global enabled → nothing pruned.
	if got := s.filterDisabledFeatureItems(context.Background(), items); len(got) != 2 {
		t.Fatalf("unattached: expected both items, got %+v", got)
	}
}
