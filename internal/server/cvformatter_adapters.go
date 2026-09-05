// Package server — cvformatter_adapters.go bridges the concrete
// *ai.Dispatcher and valkeycompat.Cmdable to the cvformatter service's
// minimal AIDispatcher and Cache interfaces. Defined here (not in
// internal/cvformatter) to avoid an import cycle and keep the
// cvformatter package decoupled from concrete ai/valkey types.
//
// Ref: japaconnect-engineering/sections/95-cv-formatter.md
//      japaconnect-engineering/adr/ADR-103-cv-formatter.md

package server

import (
	"context"
	"time"

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/cvformatter"
)

// cvFormatterAIDispatcher adapts *ai.Dispatcher to
// cvformatter.AIDispatcher. It sends the system + user prompt to the
// "content_generation" category and extracts the text/tokens/model from
// the response.
type cvFormatterAIDispatcher struct {
	dispatcher *ai.Dispatcher
}

func (a *cvFormatterAIDispatcher) Dispatch(ctx context.Context, category string, req cvformatter.AIRequest) (cvformatter.AIResponse, error) {
	resp, err := a.dispatcher.Dispatch(ctx, category, ai.Request{
		Category: category,
		Payload: map[string]any{
			"system_prompt": req.SystemPrompt,
			"prompt":        req.UserPrompt,
		},
	})
	if err != nil {
		return cvformatter.AIResponse{}, err
	}

	out := cvformatter.AIResponse{}
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

	if tokens, ok := resp.Raw["total_tokens"].(float64); ok {
		out.Tokens = int(tokens)
	} else if tokens, ok := resp.Raw["tokens_used"].(float64); ok {
		out.Tokens = int(tokens)
	}

	return out, nil
}

// cvFormatterCache adapts valkeycompat.Cmdable to cvformatter.Cache.
// INCR atomically increments the quota counter; Expire sets the TTL.
type cvFormatterCache struct {
	rdb valkeycompat.Cmdable
}

func (c *cvFormatterCache) INCR(ctx context.Context, key string) (int64, error) {
	return c.rdb.Incr(ctx, key).Result()
}

func (c *cvFormatterCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.rdb.Expire(ctx, key, ttl).Err()
}
