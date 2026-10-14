// Package ai — stream.go adds normalized token-level streaming to the
// provider gateway (ADR-130 Phase 2; §103 "Phases 2–4" amendment).
//
// Providers that support a native streaming API implement the optional
// ChatStreamer interface. Dispatcher.ChatWithToolsStream preserves the
// ChatWithTools priority/fallback order with two additions:
//
//   - A provider that fails BEFORE emitting any delta first degrades to its
//     own non-streaming ChatWithTools (covers endpoints that reject
//     stream_options or only expose single-shot completions), then the
//     dispatcher falls through to the next provider as usual.
//   - Once a delta has been emitted, a failure is terminal for the turn:
//     the client has already seen partial text, and re-dispatching would
//     emit duplicated content.
//
// The returned ChatWithToolsResponse is identical in shape to ChatWithTools
// output — full Content, accumulated ToolCalls, FinishReason, Usage — so
// the agent loop treats streamed and single-shot results interchangeably.
package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// DeltaSink receives a normalized text delta — one fragment of the
// assistant's reply as the provider emits it. Implementations forward each
// non-empty fragment exactly once, in order.
type DeltaSink func(delta string)

// ErrStreamingUnsupported marks a provider that cannot stream the given
// request (e.g. streaming exists but not for function calling). The
// dispatcher treats it like any other pre-delta failure: degrade to the
// provider's non-streaming ChatWithTools before falling through.
var ErrStreamingUnsupported = errors.New("ai: streaming not supported by this provider")

// ChatStreamer is the optional streaming contract. Providers implementing
// it use their native streaming API, forward each normalized text delta to
// sink, and return the complete ChatWithToolsResponse — full Content,
// accumulated ToolCalls (streamed tool-call fragments are reassembled
// provider-side), FinishReason, and Usage.
//
// Providers without a usable stream API (Perplexity, Devin, Cursor) simply
// don't implement this interface and stay on the single-shot path.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Phases 2–4)
type ChatStreamer interface {
	ChatStream(ctx context.Context, req ChatWithToolsRequest, sink DeltaSink) (ChatWithToolsResponse, error)
}

// ChatWithToolsStream dispatches a function-calling chat completion with
// token-level streaming. Fallback semantics mirror ChatWithTools: providers
// are tried in priority order and the first success wins. When sink is nil
// the deltas are still consumed but dropped — callers that only need the
// final response get identical behavior to ChatWithTools.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md.
func (d *Dispatcher) ChatWithToolsStream(ctx context.Context, category string, req ChatWithToolsRequest, sink DeltaSink) (ChatWithToolsResponse, error) {
	providers := d.providersForModel(category, req.Model)

	if len(providers) == 0 {
		return ChatWithToolsResponse{}, fmt.Errorf("%w for category %q", ErrNoProviders, category)
	}

	var lastErr error

	for _, p := range providers {
		streamer, ok := p.(ChatStreamer)
		if !ok {
			// Non-streaming provider — transparent single-shot path.
			resp, err := p.ChatWithTools(ctx, req)
			if err == nil {
				resp.ProviderName = p.Name()
				return resp, nil
			}

			lastErr = err
			slog.Error(
				"ai provider ChatWithTools failed, trying next",
				"provider", p.Name(),
				"category", category,
				"error", err,
			)

			continue
		}

		emitted := false
		resp, err := streamer.ChatStream(ctx, req, func(delta string) {
			if delta == "" {
				return
			}

			emitted = true

			if sink != nil {
				sink(delta)
			}
		})

		switch {
		case err == nil:
			resp.ProviderName = p.Name()
			return resp, nil

		case emitted:
			// Partial text already reached the client — falling back would
			// duplicate it. Fail the turn instead.
			return ChatWithToolsResponse{}, fmt.Errorf("ai: provider %q stream failed mid-response: %w", p.Name(), err)

		default:
			// Nothing reached the client — degrade within the same provider
			// (stream rejected/unsupported), then move to the next provider
			// per the normal fallback order.
			if !errors.Is(err, ErrStreamingUnsupported) {
				slog.Warn(
					"ai provider ChatStream failed before first delta, degrading to single-shot",
					"provider", p.Name(),
					"category", category,
					"error", err,
				)
			}

			resp, serr := p.ChatWithTools(ctx, req)
			if serr == nil {
				resp.ProviderName = p.Name()
				return resp, nil
			}

			lastErr = serr
			slog.Error(
				"ai provider ChatWithToolsStream failed, trying next",
				"provider", p.Name(),
				"category", category,
				"error", serr,
			)
		}
	}

	return ChatWithToolsResponse{}, fmt.Errorf("ai: all providers for category %q failed: %w", category, lastErr)
}

// estimateTurnTokens approximates token usage (~4 chars/token) for providers
// that report no usage while streaming. The agent loop feeds the estimate to
// CostEstimator so jc:ai:cost / jc:agent:cost stay accurate even when a
// stream carries no usage chunk.
func estimateTurnTokens(msgs []ChatMessage, reply string) (prompt, completion int) {
	for _, m := range msgs {
		prompt += (len(m.Content) + len(m.Name) + len(m.ToolCallID) + 3) / 4
		for _, tc := range m.ToolCalls {
			prompt += (len(tc.Name) + len(tc.Arguments) + 3) / 4
		}
	}

	completion = (len(reply) + 3) / 4

	return prompt, completion
}
