// Package spam — ai_classifier.go provides the AI-powered spam classification
// layer that calls the AI Gateway's "moderation" category to score content
// semantically, catching spam that rule-based layers miss.
//
// The classifier is optional: if no provider is registered for the
// "moderation" category, the layer degrades to a zero-score no-op and the
// pipeline falls back to the original rule-based behaviour.
package spam

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jedafe/japa/internal/ai"
)

// AIClassifier adapts an ai.Dispatcher to the SpamClassifier interface.
// It dispatches to the "moderation" category and parses the model's JSON
// response into a normalised score + reasons.
type AIClassifier struct {
	dispatcher *ai.Dispatcher
}

// NewAIClassifier creates an AIClassifier backed by the given dispatcher.
func NewAIClassifier(d *ai.Dispatcher) *AIClassifier {
	return &AIClassifier{dispatcher: d}
}

// ClassifySpam sends the content to the AI gateway's moderation category and
// parses the response. Returns a zero result (not an error) when no
// moderation provider is registered — the pipeline treats this as "AI layer
// unavailable, rely on rules.".
func (c *AIClassifier) ClassifySpam(ctx context.Context, content string) (AIClassificationResult, error) {
	if c == nil || c.dispatcher == nil {
		return AIClassificationResult{}, nil
	}

	if !c.dispatcher.HasCategory("moderation") {
		return AIClassificationResult{}, nil
	}

	prompt := fmt.Sprintf(`You are a content moderation classifier for a community platform.
Analyze the following user-submitted content and determine if it is spam, scam, phishing, or low-quality promotional flooding.

Content to classify:
%s

Respond with ONLY a JSON object (no markdown fences, no explanation) in this exact format:
{"score": 0.0, "is_spam": false, "reasons": []}

Rules:
- score: float between 0.0 (definitely not spam) and 1.0 (definitely spam)
- is_spam: true if score > 0.5, false otherwise
- reasons: array of short strings (max 3) explaining the classification`, content)

	resp, err := c.dispatcher.Dispatch(ctx, "moderation", ai.Request{
		Category: "moderation",
		Payload: map[string]any{
			"prompt": prompt,
		},
	})
	if err != nil {
		return AIClassificationResult{}, fmt.Errorf("ai spam classification dispatch: %w", err)
	}

	text := extractChoiceText(resp.Raw)
	if text == "" {
		return AIClassificationResult{}, nil
	}

	text = cleanJSONResponse(text)

	var result struct {
		Score   float64  `json:"score"`
		IsSpam  bool     `json:"is_spam"`
		Reasons []string `json:"reasons"`
	}

	if err := json.Unmarshal([]byte(text), &result); err != nil {
		slog.Warn("spam: failed to parse AI classification response", "error", err, "text", truncate(text, 200))
		return AIClassificationResult{}, nil
	}

	if result.Score < 0 {
		result.Score = 0
	} else if result.Score > 1 {
		result.Score = 1
	}

	return AIClassificationResult{
		Score:   result.Score,
		Reasons: result.Reasons,
	}, nil
}

// extractChoiceText pulls the assistant's message text out of an OpenAI-
// compatible choices array in the Response.Raw map.
func extractChoiceText(raw map[string]any) string {
	choices, ok := raw["choices"]
	if !ok {
		return ""
	}

	arr, ok := choices.([]any)
	if !ok || len(arr) == 0 {
		return ""
	}

	first, ok := arr[0].(map[string]any)
	if !ok {
		return ""
	}

	msg, ok := first["message"].(map[string]any)
	if !ok {
		return ""
	}

	content, _ := msg["content"].(string)

	return content
}

// cleanJSONResponse strips markdown code fences and surrounding whitespace
// so json.Unmarshal can parse the model's output directly. Models sometimes
// wrap JSON in ```json ... ``` despite instructions not to.
func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "..."
}
