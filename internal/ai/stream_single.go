// Package ai — stream_single.go provides the single-provider variant of
// Dispatcher.ChatWithToolsStream for the BYOK path: the stream →
// single-shot degrade dance against one provider, with NO cross-provider
// fallback (a BYOK turn must never silently switch to a platform key).
// Mid-stream failure stays terminal — partial text already reached the
// client, and retrying would duplicate it.
//
// Ref: docs/superpowers/specs/2026-10-19-ai-byok-design.md
package ai

import (
	"context"
	"fmt"
)

// chatWithToolsStreamOn runs a function-calling chat completion against one
// provider: native ChatStream when implemented (degrading to ChatWithTools
// when the stream fails before its first delta), single-shot otherwise.
func chatWithToolsStreamOn(ctx context.Context, p Provider, req ChatWithToolsRequest, sink DeltaSink) (ChatWithToolsResponse, error) {
	streamer, ok := p.(ChatStreamer)
	if !ok {
		resp, err := p.ChatWithTools(ctx, req)
		if err != nil {
			return ChatWithToolsResponse{}, err
		}

		resp.ProviderName = p.Name()

		return resp, nil
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
		return ChatWithToolsResponse{}, fmt.Errorf("ai: provider %q stream failed mid-response: %w", p.Name(), err)
	default:
		resp, serr := p.ChatWithTools(ctx, req)
		if serr == nil {
			resp.ProviderName = p.Name()
			return resp, nil
		}

		return ChatWithToolsResponse{}, serr
	}
}
