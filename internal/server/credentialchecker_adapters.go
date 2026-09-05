// Package server — credentialchecker_adapters.go bridges the concrete
// *ai.Dispatcher to the credentialchecker service's minimal AIDispatcher
// interface. Defined here (not in internal/credentialchecker) to avoid
// an import cycle and keep the credentialchecker package decoupled from
// concrete ai types.
//
// Ref: japaconnect-engineering/sections/20-ai-core.md
//      japaconnect-engineering/sections/94-professional-credential-checker.md
//      japaconnect-engineering/adr/ADR-088-destination-wiki.md

package server

import (
	"context"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/credentialchecker"
)

// credentialCheckerAIDispatcher adapts *ai.Dispatcher to
// credentialchecker.AIDispatcher. It sends the system + user prompt to
// the "content_generation" category and extracts the text/model from
// the response.
type credentialCheckerAIDispatcher struct {
	dispatcher *ai.Dispatcher
}

func (a *credentialCheckerAIDispatcher) HasCategory(category string) bool {
	return a.dispatcher.HasCategory(category)
}

func (a *credentialCheckerAIDispatcher) Dispatch(ctx context.Context, category string, req credentialchecker.AIRequest) (credentialchecker.AIResponse, error) {
	resp, err := a.dispatcher.Dispatch(ctx, category, ai.Request{
		Category: category,
		Payload: map[string]any{
			"system_prompt": req.SystemPrompt,
			"prompt":        req.UserPrompt,
		},
	})
	if err != nil {
		return credentialchecker.AIResponse{}, err
	}

	out := credentialchecker.AIResponse{}
	if text, ok := resp.Raw["text"].(string); ok {
		out.Text = text
	} else if text, ok := resp.Raw["content"].(string); ok {
		out.Text = text
	} else if text, ok := resp.Raw["output"].(string); ok {
		out.Text = text
	}

	if model, ok := resp.Raw["model"].(string); ok {
		out.Model = model
	}

	return out, nil
}
