package trending

import (
	"context"
	"testing"

	"github.com/jedafe/japa/internal/features"
)

type stubTrendingChecker struct {
	enabled bool
	eval    features.EvalResult
}

func (s stubTrendingChecker) IsEnabled(context.Context, string) bool { return s.enabled }
func (s stubTrendingChecker) EvalFlag(context.Context, string, features.CallerScope) features.EvalResult {
	return s.eval
}

func TestFeatureVisibleScopeSemantics(t *testing.T) {
	scoped := features.WithScope(context.Background(), features.CallerScope{Surface: "main"})
	h := &Handlers{flags: stubTrendingChecker{enabled: true, eval: features.EvalResult{Enabled: true, Visible: false}}}

	// Attached scope: hidden-on-surface → pruned from the projection.
	if h.featureVisible(scoped, "story.enabled") {
		t.Error("scoped hidden flag must not be visible on request paths")
	}

	// Unattached ctx: global enabled → shown (worker semantics).
	if !h.featureVisible(context.Background(), "story.enabled") {
		t.Error("unattached ctx must use the global toggle")
	}

	// Nil checker → visible (fail-open, tests/dev).
	if !(&Handlers{}).featureVisible(scoped, "story.enabled") {
		t.Error("nil checker must be visible")
	}
}
