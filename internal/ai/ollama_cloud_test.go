// Package ai — ollama_cloud_test.go verifies the OpenAI-compatible Ollama Cloud
// endpoint through the same provider used by the dispatcher.
//
// It is skipped unless JC_OLLAMA_CLOUD_API_KEY is set, so it does not fail in
// normal CI. To run it locally:
//
//	export JC_OLLAMA_CLOUD_API_KEY=your-key
//	go test ./internal/ai -run TestOllamaCloud -v
package ai

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestOllamaCloudProviderChat(t *testing.T) {
	apiKey := os.Getenv("JC_OLLAMA_CLOUD_API_KEY")
	if apiKey == "" {
		t.Skip("JC_OLLAMA_CLOUD_API_KEY not set; skipping live Ollama Cloud test")
	}

	p := NewOpenAIProvider("ollama-cloud", apiKey, "https://ollama.com/v1", "gpt-oss:20b-cloud")
	if p == nil {
		t.Fatal("NewOpenAIProvider returned nil")
	}

	ctx := context.Background()
	resp, err := p.Send(ctx, Request{
		Category: "content_generation",
		Payload: map[string]any{
			"prompt": "Say 'Ollama Cloud is reachable' and nothing else.",
		},
	})
	if err != nil {
		t.Fatalf("Ollama Cloud Send() failed: %v", err)
	}

	if resp.Status != "success" {
		t.Fatalf("Ollama Cloud returned non-success status: %s", resp.Status)
	}

	// Extract model, usage and text from the provider's Raw response.
	model, _ := resp.Raw["model"].(string)
	var text string
	if choices, ok := resp.Raw["choices"].([]openAIChoice); ok && len(choices) > 0 {
		text = choices[0].Message.Content
	}

	if text == "" {
		t.Fatal("Ollama Cloud returned empty response text")
	}

	usage, _ := resp.Raw["usage"].(openAIUsage)
	totalTokens := usage.PromptTokens + usage.CompletionTokens

	t.Logf("Ollama Cloud response: provider=%s model=%s tokens=%d text=%q",
		p.Name(), model, totalTokens, text)

	// Token usage must be reported for cost monitoring. The OpenAI-compatible
	// endpoint should return prompt_tokens and completion_tokens in usage.
	if totalTokens <= 0 {
		t.Fatalf("Ollama Cloud returned zero token usage: %+v", usage)
	}

	// Verify the cost estimator can resolve a price for this provider/model.
	estimator := NewCostEstimator(&defaultCostSettings{})
	cost := estimator.Estimate(ctx, pgtype.UUID{}, p.Name(), model, totalTokens)
	if cost <= 0 {
		t.Fatalf("CostEstimator returned non-positive cost: %f", cost)
	}
}

// defaultCostSettings is a minimal SettingsReader for the cost estimator test.
type defaultCostSettings struct{}

func (defaultCostSettings) GetStringOr(context.Context, pgtype.UUID, string, string) string { return "{}" }
func (defaultCostSettings) GetIntOr(context.Context, pgtype.UUID, string, int) int         { return 0 }
func (s defaultCostSettings) GetFloatOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback float64) float64 {
	if key == "ai.cost_per_1k_tokens_usd" {
		return 0.002
	}
	return fallback
}
func (defaultCostSettings) GetBoolOr(context.Context, pgtype.UUID, string, bool) bool { return false }
