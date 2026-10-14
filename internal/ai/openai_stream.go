// Package ai — openai_stream.go implements token-level streaming for
// OpenAI-compatible chat-completions providers (ADR-130 Phase 2). The
// reader is shared by OpenAIProvider (openai, groq, ollama-cloud, custom)
// and XiaomiProvider — both speak the same chunk format; only the request
// body and auth header differ.
//
// Wire contract: POST {endpoint} with stream:true returns an SSE stream of
// `data: {chunk}\n\n` lines terminated by `data: [DONE]`. Each chunk's
// choices[0].delta carries either a content fragment or a tool_calls
// fragment (keyed by delta.tool_calls[].index — arguments arrive split
// across chunks and are concatenated). With stream_options.include_usage
// the terminal chunk carries a `usage` object.
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

// openAIStreamOptions requests usage accounting on the terminal stream
// chunk (stream_options.include_usage — ADR-130 Phase 2).
type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// openAIStreamChunk is a single SSE data payload from an OpenAI-compatible
// streaming completion.
type openAIStreamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			Reasoning        string `json:"reasoning"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage"`
}

// streamChatCompletions executes a streaming chat-completions POST and
// parses the OpenAI-compatible SSE chunk stream. body is the marshaled
// request (with stream:true); headers carry the provider's auth. Text
// deltas are forwarded to sink; tool-call fragments are reassembled into
// ToolCall entries on the returned response.
func streamChatCompletions(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	headers map[string]string,
	body []byte,
	sink DeltaSink,
) (ChatWithToolsResponse, error) {
	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("creating stream request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := client.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("stream request failed: %w", err)
	}

	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
		return ChatWithToolsResponse{}, fmt.Errorf("stream API error (status %d): %s", httpResp.StatusCode, truncate(string(respBody), 500))
	}

	var (
		content   strings.Builder
		reasoning strings.Builder
		model     string
		refID     string
		finish    string
		usage     TokenUsage
		sawDone   bool
	)

	// Streamed tool calls arrive as fragments keyed by their index in the
	// delta's tool_calls array; accumulate in first-seen order.
	type toolAccum struct {
		order int
		tc    openAIToolCall
	}
	toolAccums := map[int]*toolAccum{}
	nextOrder := 0

	reader := bufio.NewReaderSize(httpResp.Body, 8192)

	for {
		line, readErr := reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")

		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

			if payload == "[DONE]" {
				sawDone = true
				break
			}

			if payload != "" {
				var chunk openAIStreamChunk
				if jerr := json.Unmarshal([]byte(payload), &chunk); jerr != nil {
					slog.Warn("ai stream: skipping unparseable chunk", "error", jerr)
				} else {
					if chunk.ID != "" {
						refID = chunk.ID
					}
					if chunk.Model != "" {
						model = chunk.Model
					}
					if chunk.Usage != nil {
						usage = TokenUsage{
							PromptTokens:     chunk.Usage.PromptTokens,
							CompletionTokens: chunk.Usage.CompletionTokens,
							TotalTokens:      chunk.Usage.TotalTokens,
						}
					}

					for _, choice := range chunk.Choices {
						if choice.FinishReason != "" {
							finish = choice.FinishReason
						}

						d := choice.Delta
						switch {
						case d.Content != "":
							content.WriteString(d.Content)
							if sink != nil {
								sink(d.Content)
							}
						case d.Reasoning != "":
							reasoning.WriteString(d.Reasoning)
						case d.ReasoningContent != "":
							reasoning.WriteString(d.ReasoningContent)
						}

						for _, tc := range d.ToolCalls {
							acc, exists := toolAccums[tc.Index]
							if !exists {
								acc = &toolAccum{order: nextOrder}
								nextOrder++
								toolAccums[tc.Index] = acc
							}
							if tc.ID != "" {
								acc.tc.ID = tc.ID
							}
							if tc.Function.Name != "" && acc.tc.Function.Name == "" {
								acc.tc.Function.Name = tc.Function.Name
							}
							acc.tc.Function.Arguments += tc.Function.Arguments
						}
					}
				}
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				// Some compatible servers close without [DONE]; accept the
				// stream only when a finish_reason was observed.
				if finish == "" {
					return ChatWithToolsResponse{}, errors.New("stream ended before a finish_reason was received")
				}
				break
			}
			return ChatWithToolsResponse{}, fmt.Errorf("reading stream: %w", readErr)
		}
	}

	result := ChatWithToolsResponse{
		Content:      content.String(),
		FinishReason: finish,
		Model:        model,
		ProviderRef:  refID,
		Usage:        usage,
	}

	// Reasoning-model fallback: providers that answer in the reasoning field
	// (StepFun-style, MiMo) leave content empty — mirror the Send/ExtractText
	// fallback so the loop still gets the reply text.
	if result.Content == "" && reasoning.Len() > 0 {
		result.Content = reasoning.String()
	}

	if result.FinishReason == "" && sawDone {
		result.FinishReason = "stop"
	}

	ordered := make([]int, 0, len(toolAccums))
	for idx := range toolAccums {
		ordered = append(ordered, idx)
	}
	// Sort by first-seen order so repeated tool calls keep arrival order.
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if toolAccums[ordered[j]].order < toolAccums[ordered[i]].order {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}
	for _, idx := range ordered {
		tc := toolAccums[idx].tc
		args := json.RawMessage(tc.Function.Arguments)
		if len(args) == 0 || string(args) == "null" {
			args = json.RawMessage("{}")
		}

		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return result, nil
}

// ChatStream implements ChatStreamer for OpenAI-compatible providers:
// POST /chat/completions with stream:true + stream_options.include_usage.
// Text deltas flow to sink; the response carries the accumulated reply,
// reassembled tool calls, and the terminal usage chunk.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Phases 2–4).
func (p *OpenAIProvider) ChatStream(ctx context.Context, req ChatWithToolsRequest, sink DeltaSink) (ChatWithToolsResponse, error) {
	if p.apiKey == "" {
		return ChatWithToolsResponse{}, errors.New("openai: API key not configured")
	}

	if len(req.Messages) == 0 {
		return ChatWithToolsResponse{}, errors.New("openai: at least one message is required")
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	chatReq := openAIToolsChatRequest{
		Model:         model,
		Messages:      toOpenAIToolsMessages(req.Messages),
		Tools:         toOpenAIToolDefs(req.Tools),
		Stream:        true,
		StreamOptions: &openAIStreamOptions{IncludeUsage: true},
	}
	if isReasoningModel(model) || p.useMaxCompletionTokens {
		chatReq.MaxCompletionTokens = req.MaxTokens
	} else {
		chatReq.MaxTokens = req.MaxTokens
	}

	if !isReasoningModel(model) {
		chatReq.Temperature = req.Temperature
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("openai: marshaling stream request: %w", err)
	}

	resp, err := streamChatCompletions(
		ctx,
		p.streamClient,
		p.baseURL+"/chat/completions",
		map[string]string{
			"Authorization": "Bearer " + p.apiKey,
			"HTTP-Referer":  "https://japaconnect.com",
			"X-Title":       "JapaConnect",
		},
		body,
		sink,
	)
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("openai: %w", err)
	}

	slog.Info(
		"openai: stream completed",
		"provider", p.name,
		"model", resp.Model,
		"finish_reason", resp.FinishReason,
		"tool_calls", len(resp.ToolCalls),
		"total_tokens", resp.Usage.TotalTokens,
	)

	return resp, nil
}
