package jobs

import (
	"context"
	"testing"

	"github.com/jedafe/japa/internal/features"
)

// stubJobChecker satisfies FeatureChecker with canned EvalFlag results.
type stubJobChecker struct {
	enabled bool
	eval    features.EvalResult
}

func (s stubJobChecker) IsEnabled(_ context.Context, _ string) bool { return s.enabled }
func (s stubJobChecker) EvalFlag(_ context.Context, _ string, _ features.CallerScope) features.EvalResult {
	return s.eval
}

func TestIsEnabledFor(t *testing.T) {
	tests := []struct {
		name    string
		checker FeatureChecker
		want    bool
	}{
		{"enabled and visible → allowed", stubJobChecker{eval: features.EvalResult{Enabled: true, Visible: true}}, true},
		{"enabled but hidden → still serves (Gate parity)", stubJobChecker{eval: features.EvalResult{Enabled: true, Visible: false}}, true},
		{"disabled globally → denied", stubJobChecker{eval: features.EvalResult{Enabled: false, Visible: false}}, false},
		{"nil checker + nil settings → allowed (fail-open default)", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewService(nil, nil)
			s.SetFeatureChecker(tt.checker)
			if got := s.IsEnabledFor(context.Background(), features.CallerScope{Surface: "main"}); got != tt.want {
				t.Errorf("IsEnabledFor = %v; want %v", got, tt.want)
			}
		})
	}
}
