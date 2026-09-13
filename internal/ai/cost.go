// Package ai — cost.go provides cost estimation for AI provider calls.
//
// The CostEstimator looks up per-provider, per-model prices from system_settings
// and falls back to a global default so usage monitoring reflects real spend
// (Section 20, ADR-111 §5.10).
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// defaultCostPer1kTokensUSD is the fallback price when no setting is configured.
// It intentionally matches the cheap-model range (MiMo Flash, GPT-4o-mini,
// Llama 8B) so the default monitoring is conservative.
const defaultCostPer1kTokensUSD = 0.002

// CostEstimator estimates USD cost from token usage using system_settings.
type CostEstimator struct {
	settings SettingsReader
}

// NewCostEstimator creates a CostEstimator backed by the given settings reader.
// A nil settings reader falls back to defaultCostPer1kTokensUSD.
func NewCostEstimator(settings SettingsReader) *CostEstimator {
	return &CostEstimator{settings: settings}
}

// Estimate returns the cost in USD for totalTokens, looking up per-provider
// and per-model prices before falling back to global defaults.
func (e *CostEstimator) Estimate(ctx context.Context, tenantID pgtype.UUID, provider, model string, totalTokens int) float64 {
	if totalTokens <= 0 {
		return 0
	}

	price := e.pricePer1k(ctx, tenantID, provider, model)

	return float64(totalTokens) * price / 1000.0
}

// pricePer1k resolves the USD cost per 1,000 tokens for a provider/model.
// Resolution order (most specific wins):
//  1. ai.provider.{provider}.{model}.cost_per_1k_tokens_usd (SQL/legacy flat key)
//  2. ai.provider_cost_overrides["provider.model"] (Control Panel JSON map)
//  3. ai.provider.{provider}.cost_per_1k_tokens_usd (SQL/legacy flat key)
//  4. ai.provider_cost_overrides["provider"] (Control Panel JSON map)
//  5. ai.cost_per_1k_tokens_usd
//  6. agent.cost_per_1k_tokens (legacy fallback, kept for backward compatibility)
//  7. defaultCostPer1kTokensUSD
func (e *CostEstimator) pricePer1k(ctx context.Context, tenantID pgtype.UUID, provider, model string) float64 {
	if e.settings == nil {
		return defaultCostPer1kTokensUSD
	}

	overrides := e.loadProviderCostOverrides(ctx, tenantID)

	if model != "" && provider != "" {
		key := fmt.Sprintf("ai.provider.%s.%s.cost_per_1k_tokens_usd", provider, model)

		v := e.settings.GetFloatOr(ctx, tenantID, key, -1)
		if v >= 0 {
			return v
		}

		if v, ok := overrides[provider+"."+model]; ok && v >= 0 {
			return v
		}
	}

	if provider != "" {
		key := fmt.Sprintf("ai.provider.%s.cost_per_1k_tokens_usd", provider)

		v := e.settings.GetFloatOr(ctx, tenantID, key, -1)
		if v >= 0 {
			return v
		}

		if v, ok := overrides[provider]; ok && v >= 0 {
			return v
		}
	}

	// Global AI cost default.
	v := e.settings.GetFloatOr(ctx, tenantID, "ai.cost_per_1k_tokens_usd", -1)
	if v >= 0 {
		return v
	}

	// Legacy agent setting for backward compatibility.
	return e.settings.GetFloatOr(ctx, tenantID, "agent.cost_per_1k_tokens", defaultCostPer1kTokensUSD)
}

// loadProviderCostOverrides parses the ai.provider_cost_overrides JSON map.
// Keys are "provider" or "provider.model"; values are USD per 1,000 tokens.
// The map is cached on the CostEstimator for the lifetime of the estimate call
// (the caller holds a context, so the same request does not parse it twice).
func (e *CostEstimator) loadProviderCostOverrides(ctx context.Context, tenantID pgtype.UUID) map[string]float64 {
	raw := e.settings.GetStringOr(ctx, pgtype.UUID{}, "ai.provider_cost_overrides", "{}")
	if raw == "" {
		raw = "{}"
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		slog.Warn("ai: failed to parse ai.provider_cost_overrides", "error", err)
		return nil
	}

	overrides := make(map[string]float64, len(out))

	for k, v := range out {
		switch n := v.(type) {
		case float64:
			overrides[k] = n
		case string:
			f, err := strconv.ParseFloat(n, 64)
			if err == nil {
				overrides[k] = f
			}
		}
	}

	return overrides
}
