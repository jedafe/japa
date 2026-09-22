package server

import (
	"context"
	"time"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/sopgenerator"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// sopAIDispatcher adapts *ai.Dispatcher to the sopgenerator.AIDispatcher
// interface (§110 — §20 AI Gateway seam).
type sopAIDispatcher struct {
	dispatcher *ai.Dispatcher
}

func (a *sopAIDispatcher) Dispatch(ctx context.Context, category string, req sopgenerator.AIRequest) (sopgenerator.AIResponse, error) {
	resp, err := a.dispatcher.Dispatch(ctx, category, ai.Request{
		Category: category,
		Payload: map[string]any{
			"system_prompt": req.SystemPrompt,
			"prompt":        req.UserPrompt,
		},
	})
	if err != nil {
		return sopgenerator.AIResponse{}, err
	}

	out := sopgenerator.AIResponse{}
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
	}
	return out, nil
}

// sopQuotaCache adapts the Valkey client to the sopgenerator.Cache
// interface for jc:sop_quota:{user_id}:{YYYY-MM}.
type sopQuotaCache struct {
	rdb valkeycompat.Cmdable
}

func (c *sopQuotaCache) INCR(ctx context.Context, key string) (int64, error) {
	return c.rdb.Incr(ctx, key).Result()
}

func (c *sopQuotaCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.rdb.Expire(ctx, key, ttl).Err()
}
