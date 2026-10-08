package server

import (
	"context"
	"time"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/crscalculator"
	"github.com/jedafe/japa/internal/ecaguide"
	"github.com/jedafe/japa/internal/pofcalculator"
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
	return sopgenerator.AIResponse{Text: ai.ExtractText(resp), Model: ai.ExtractModel(resp), Tokens: ai.ExtractTokens(resp)}, nil
}

// sopQuotaCache adapts the Valkey client to the sopgenerator.Cache
// interface for jc:sop_quota:{user_id}:{YYYY-MM}.
type sopQuotaCache struct {
	rdb valkeycompat.Cmdable
}

func (c *sopQuotaCache) INCR(ctx context.Context, key string) (int64, error) {
	return c.rdb.Incr(ctx, key).Result()
}

func (c *sopQuotaCache) DECR(ctx context.Context, key string) (int64, error) {
	return c.rdb.Decr(ctx, key).Result()
}

func (c *sopQuotaCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.rdb.Expire(ctx, key, ttl).Err()
}

// dispatchToolAI is the shared §20 AI Gateway call for the
// §§107–110 Content Agent draft adapters (tool:generate_draft task).
func dispatchToolAI(ctx context.Context, d *ai.Dispatcher, category, system, user string) (text, model string, err error) {
	resp, err := d.Dispatch(ctx, category, ai.Request{
		Category: category,
		Payload: map[string]any{
			"system_prompt": system,
			"prompt":        user,
		},
	})
	if err != nil {
		return "", "", err
	}
	return ai.ExtractText(resp), ai.ExtractModel(resp), nil
}

// crsAIDispatcher satisfies crscalculator.AIDispatcher (§107).
type crsAIDispatcher struct{ dispatcher *ai.Dispatcher }

func (a *crsAIDispatcher) Dispatch(ctx context.Context, category string, req crscalculator.AIRequest) (crscalculator.AIResponse, error) {
	text, model, err := dispatchToolAI(ctx, a.dispatcher, category, req.SystemPrompt, req.UserPrompt)
	return crscalculator.AIResponse{Text: text, Model: model}, err
}

// pofAIDispatcher satisfies pofcalculator.AIDispatcher (§108).
type pofAIDispatcher struct{ dispatcher *ai.Dispatcher }

func (a *pofAIDispatcher) Dispatch(ctx context.Context, category string, req pofcalculator.AIRequest) (pofcalculator.AIResponse, error) {
	text, model, err := dispatchToolAI(ctx, a.dispatcher, category, req.SystemPrompt, req.UserPrompt)
	return pofcalculator.AIResponse{Text: text, Model: model}, err
}

// ecaAIDispatcher satisfies ecaguide.AIDispatcher (§109).
type ecaAIDispatcher struct{ dispatcher *ai.Dispatcher }

func (a *ecaAIDispatcher) Dispatch(ctx context.Context, category string, req ecaguide.AIRequest) (ecaguide.AIResponse, error) {
	text, model, err := dispatchToolAI(ctx, a.dispatcher, category, req.SystemPrompt, req.UserPrompt)
	return ecaguide.AIResponse{Text: text, Model: model}, err
}
