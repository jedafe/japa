package video

import (
	"context"
	"testing"

	"github.com/jedafe/japa/internal/features"
)

// stubVideoChecker satisfies FeatureChecker with canned results.
type stubVideoChecker struct {
	enabled bool
	eval    features.EvalResult
}

func (s stubVideoChecker) IsEnabled(_ context.Context, _ string) bool { return s.enabled }
func (s stubVideoChecker) EvalFlag(_ context.Context, _ string, _ features.CallerScope) features.EvalResult {
	return s.eval
}

func TestIsEnabledForRequest(t *testing.T) {
	newSvc := func(c FeatureChecker) *Service {
		s := NewService(nil, nil, nil, nil)
		s.SetFeatureChecker(c)
		return s
	}
	scopedCtx := func() context.Context {
		return features.WithScope(context.Background(), features.CallerScope{Surface: "main"})
	}

	t.Run("attached scope uses EvalFlag — enabled+visible allowed", func(t *testing.T) {
		s := newSvc(stubVideoChecker{eval: features.EvalResult{Enabled: true, Visible: true}})
		if !s.isEnabledForRequest(scopedCtx()) {
			t.Error("expected enabled for in-scope caller")
		}
	})

	t.Run("attached scope honours surface-disabled eval", func(t *testing.T) {
		s := newSvc(stubVideoChecker{enabled: true, eval: features.EvalResult{Enabled: false, Visible: false}})
		if s.isEnabledForRequest(scopedCtx()) {
			t.Error("scoped-disabled must deny on request paths")
		}
	})

	t.Run("attached scope — hidden-but-enabled still serves (Gate parity)", func(t *testing.T) {
		s := newSvc(stubVideoChecker{enabled: true, eval: features.EvalResult{Enabled: true, Visible: false}})
		if !s.isEnabledForRequest(scopedCtx()) {
			t.Error("a hidden-but-enabled flag must serve routes, matching features.Gate")
		}
	})

	t.Run("unattached ctx falls back to global IsEnabled (worker semantics)", func(t *testing.T) {
		s := newSvc(stubVideoChecker{enabled: true, eval: features.EvalResult{Enabled: false, Visible: false}})
		if !s.isEnabledForRequest(context.Background()) {
			t.Error("unattached ctx must use the global toggle")
		}
	})

	t.Run("unattached ctx honours global disable", func(t *testing.T) {
		s := newSvc(stubVideoChecker{enabled: false})
		if s.isEnabledForRequest(context.Background()) {
			t.Error("globally disabled must deny")
		}
	})
}
