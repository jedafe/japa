// Package ai — anthropic_stream.go implements token-level streaming for the
// Anthropic Messages API (ADR-130 Phase 2).
//
// Wire contract: POST /v1/messages with stream:true returns an SSE stream
// of `event:`/`data:` pairs. Each data payload carries a `type` field:
//
//   - message_start       — {message:{id, model, usage:{input_tokens}}}
//   - content_block_start — {index, content_block:{type:"text"|"tool_use", id, name}}
//   - content_block_delta — {index, delta:{type:"text_delta",text} |
//     {type:"input_json_delta",partial_json} | ...}
//   - content_block_stop  — {index}
//   - message_delta       — {delta:{stop_reason}, usage:{output_tokens}}
//   - message_stop / ping — stream end / keepalive
//   - error               — {error:{type,message}} — mid-stream failure
//
// Text deltas flow to sink; tool_use blocks are reassembled by index into
// ToolCall entries (partial_json fragments concatenated, parsed lazily —
// the executor validates the raw JSON like every other provider).
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// anthropicStreamEvent is the union shape of every SSE data payload.
type anthropicStreamEvent struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message *struct {
		ID    string         `json:"id"`
		Model string         `json:"model"`
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage *anthropicUsage `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// anthropicStreamBlock accumulates one content block across deltas.
type anthropicStreamBlock struct {
	typ   string
	id    string
	name  string
	text  strings.Builder
	input strings.Builder
}

// ChatStream implements ChatStreamer for Anthropic's native Messages API
// (ADR-130 Phase 2). Usage is reconstructed from message_start (input) +
// message_delta (output) since Anthropic reports them separately.
func (p *AnthropicProvider) ChatStream(ctx context.Context, req ChatWithToolsRequest, sink DeltaSink) (ChatWithToolsResponse, error) {
	if p.apiKey == "" {
		return ChatWithToolsResponse{}, errors.New("anthropic: API key not configured")
	}

	if len(req.Messages) == 0 {
		return ChatWithToolsResponse{}, errors.New("anthropic: at least one message is required")
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	systemPrompt, msgs := toAnthropicToolsMessages(req.Messages)

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	chatReq := anthropicToolsRequest{
		Model:       model,
		Messages:    msgs,
		System:      systemPrompt,
		Tools:       toAnthropicToolDefs(req.Tools),
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("anthropic: marshaling stream request: %w", err)
	}

	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("anthropic: creating stream request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	if p.usesBearerAuth() {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	} else {
		httpReq.Header.Set("x-api-key", p.apiKey)
		httpReq.Header.Set("anthropic-version", AnthropicAPIVersion)
	}

	httpResp, err := p.streamClient.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("anthropic: stream request failed: %w", err)
	}

	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
		return ChatWithToolsResponse{}, fmt.Errorf("anthropic: stream API error (status %d): %s", httpResp.StatusCode, truncate(string(respBody), 500))
	}

	var (
		content    strings.Builder
		model2     string
		refID      string
		stopReason string
		inputTok   int
		outputTok  int
	)

	blocks := map[int]*anthropicStreamBlock{}
	blockOrder := []int{}

	reader := bufio.NewReaderSize(httpResp.Body, 8192)

	for {
		line, readErr := reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")

		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload != "" {
				var ev anthropicStreamEvent
				if jerr := json.Unmarshal([]byte(payload), &ev); jerr != nil {
					slog.Warn("anthropic stream: skipping unparseable event", "error", jerr)
				} else {
					switch ev.Type {
					case "message_start":
						if ev.Message != nil {
							refID = ev.Message.ID
							model2 = ev.Message.Model
							inputTok = ev.Message.Usage.InputTokens
							outputTok = ev.Message.Usage.OutputTokens
						}

					case "content_block_start":
						if ev.ContentBlock != nil {
							b := &anthropicStreamBlock{typ: ev.ContentBlock.Type, id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
							blocks[ev.Index] = b
							blockOrder = append(blockOrder, ev.Index)
						}

					case "content_block_delta":
						b := blocks[ev.Index]
						if b != nil && ev.Delta != nil {
							switch ev.Delta.Type {
							case "text_delta":
								b.text.WriteString(ev.Delta.Text)
								content.WriteString(ev.Delta.Text)
								if sink != nil {
									sink(ev.Delta.Text)
								}
							case "input_json_delta":
								b.input.WriteString(ev.Delta.PartialJSON)
							}
						}

					case "message_delta":
						if ev.Delta != nil && ev.Delta.StopReason != "" {
							stopReason = ev.Delta.StopReason
						}
						if ev.Usage != nil && ev.Usage.OutputTokens > 0 {
							outputTok = ev.Usage.OutputTokens
						}

					case "message_stop":
						// Terminal event — loop ends on EOF/next line.

					case "error":
						msg := "stream error"
						if ev.Error != nil && ev.Error.Message != "" {
							msg = ev.Error.Message
						}
						return ChatWithToolsResponse{}, fmt.Errorf("anthropic: %s", msg)

					default:
						// ping and unknown event types are ignored.
					}
				}
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return ChatWithToolsResponse{}, fmt.Errorf("anthropic: reading stream: %w", readErr)
		}
	}

	// A well-formed stream always ends with message_delta{stop_reason} →
	// message_stop. EOF without one means the connection dropped mid-turn —
	// treat the accumulated text as truncated, not complete (same strictness
	// as the OpenAI-compatible reader's missing-finish_reason error).
	if stopReason == "" {
		return ChatWithToolsResponse{}, errors.New("anthropic: stream ended before a stop_reason was received")
	}

	result := ChatWithToolsResponse{
		Content:      content.String(),
		FinishReason: anthropicStopReasonToFinish(stopReason),
		Model:        model2,
		ProviderRef:  refID,
		Usage: TokenUsage{
			PromptTokens:     inputTok,
			CompletionTokens: outputTok,
			TotalTokens:      inputTok + outputTok,
		},
	}

	for _, idx := range blockOrder {
		b := blocks[idx]
		if b == nil || b.typ != "tool_use" {
			continue
		}
		args := json.RawMessage(b.input.String())
		if len(args) == 0 || string(args) == "null" || !json.Valid(args) {
			args = json.RawMessage("{}")
		}
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:        b.id,
			Name:      b.name,
			Arguments: args,
		})
	}

	slog.Info(
		"anthropic: stream completed",
		"provider", p.name,
		"model", result.Model,
		"stop_reason", stopReason,
		"tool_calls", len(result.ToolCalls),
		"input_tokens", inputTok,
		"output_tokens", outputTok,
	)

	return result, nil
}
