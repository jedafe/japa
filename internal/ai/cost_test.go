// Package ai — cost_test.go covers the CostEstimator price resolution.
package ai

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// stubCostSettingsReader is a minimal SettingsReader for cost tests.
type stubCostSettingsReader struct {
	floats  map[string]float64
	strings map[string]string
}

func (s *stubCostSettingsReader) GetStringOr(_ context.Context, _ pgtype.UUID, key, fallback string) string {
	if s.strings == nil {
		return fallback
	}
	if v, ok := s.strings[key]; ok {
		return v
	}
	return fallback
}
func (s *stubCostSettingsReader) GetIntOr(_ context.Context, _ pgtype.UUID, _ string, fallback int) int {
	return fallback
}
func (s *stubCostSettingsReader) GetFloatOr(_ context.Context, _ pgtype.UUID, key string, fallback float64) float64 {
	if v, ok := s.floats[key]; ok {
		return v
	}
	return fallback
}
func (s *stubCostSettingsReader) GetBoolOr(_ context.Context, _ pgtype.UUID, _ string, fallback bool) bool {
	return fallback
}

func TestCostEstimatorResolution(t *testing.T) {
	t.Parallel()

	settings := &stubCostSettingsReader{
		floats: map[string]float64{
			"ai.provider.xiaomi.mimo-v2-flash.cost_per_1k_tokens_usd": 0.0003,
			"ai.provider.openai.cost_per_1k_tokens_usd":               0.001,
			"ai.cost_per_1k_tokens_usd":                               0.002,
			"agent.cost_per_1k_tokens":                                0.004,
		},
	}

	ctx := context.Background()
	var tenantID pgtype.UUID

	tests := []struct {
		name     string
		provider string
		model    string
		want     float64
	}{
		{
			name:     "model-specific override wins",
			provider: "xiaomi",
			model:    "mimo-v2-flash",
			want:     0.0003,
		},
		{
			name:     "provider override wins when no model match",
			provider: "openai",
			model:    "gpt-4o",
			want:     0.001,
		},
		{
			name:     "global AI default wins when provider unknown",
			provider: "unknown",
			model:    "unknown",
			want:     0.002,
		},
	}

	estimator := NewCostEstimator(settings)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := estimator.Estimate(ctx, tenantID, tt.provider, tt.model, 1000)
			if got != tt.want {
				t.Errorf("Estimate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCostEstimatorFallbacks(t *testing.T) {
	t.Parallel()

	// No settings: should use the hardcoded default.
	estimator := NewCostEstimator(nil)
	ctx := context.Background()
	var tenantID pgtype.UUID

	got := estimator.Estimate(ctx, tenantID, "xiaomi", "mimo-v2-flash", 1000)
	if got != defaultCostPer1kTokensUSD {
		t.Errorf("Estimate() with nil settings = %v, want %v", got, defaultCostPer1kTokensUSD)
	}

	// Settings missing all AI keys but has legacy agent key.
	settings := &stubCostSettingsReader{floats: map[string]float64{
		"agent.cost_per_1k_tokens": 0.004,
	}}
	estimator = NewCostEstimator(settings)
	got = estimator.Estimate(ctx, tenantID, "openai", "gpt-4o-mini", 1000)
	if got != 0.004 {
		t.Errorf("Estimate() with legacy agent key = %v, want 0.004", got)
	}
}

func TestCostEstimatorProviderCostOverrides(t *testing.T) {
	t.Parallel()

	settings := &stubCostSettingsReader{
		floats: map[string]float64{
			"ai.provider.xiaomi.cost_per_1k_tokens_usd": 0.0005,
			"ai.cost_per_1k_tokens_usd":                 0.002,
		},
		strings: map[string]string{
			"ai.provider_cost_overrides": `{"xiaomi.mimo-v2-flash":"0.0003","openai.gpt-4o-mini":"0.0001","openai":"0.0002"}`,
		},
	}

	ctx := context.Background()
	var tenantID pgtype.UUID
	estimator := NewCostEstimator(settings)

	tests := []struct {
		name     string
		provider string
		model    string
		want     float64
	}{
		{
			name:     "JSON model override wins over flat provider key",
			provider: "xiaomi",
			model:    "mimo-v2-flash",
			want:     0.0003,
		},
		{
			name:     "flat model key wins over JSON model override",
			provider: "openai",
			model:    "gpt-4o-mini",
			want:     0.0001, // JSON wins because no flat model key
		},
		{
			name:     "JSON provider fallback",
			provider: "openai",
			model:    "unknown-model",
			want:     0.0002,
		},
		{
			name:     "flat provider key wins over JSON provider fallback",
			provider: "xiaomi",
			model:    "unknown-model",
			want:     0.0005,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := estimator.Estimate(ctx, tenantID, tt.provider, tt.model, 1000)
			if got != tt.want {
				t.Errorf("Estimate() = %v, want %v", got, tt.want)
			}
		})
	}
}
