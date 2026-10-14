// Package mcp — tools_chat.go implements the synthetic `japa.chat` tool
// (§119, ADR-130 Phase 3): the unified Japa AI engine exposed as a unary
// MCP tool so JSON-RPC clients can converse without speaking SSE.
//
// The tool is JWT-only — API-key and OAuth callers carry the synthetic
// apiKeyCallerKey context, so they never see it in tools/list and
// tools/call denies them via toolDeniedWithTier before it is dispatched.
// The unified admission gate (ai.context.use OR agent.use, feature flags,
// per-user tool prefs) all apply inside the engine — this surface adds no
// permission surface of its own.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/auth"
)

// japaChatToolName is the synthetic tool name — deliberately not an ai_tools
// row so it never appears in the REST catalog or the admin tool registry.
const japaChatToolName = "japa.chat"

// japaChatDescriptor describes japa.chat for tools/list. JWT callers only —
// the caller check lives in handleToolsList.
func (s *Server) japaChatDescriptor() mcpTool {
	return mcpTool{
		Name:        japaChatToolName,
		Title:       "Japa Chat",
		Description: "Send a message to the Japa AI assistant and get its reply. Pass conversation_id from a previous call's result to continue the same thread.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string", "description": "The user's message"},
				"conversation_id": map[string]any{
					"type":        "string",
					"description": "Thread id returned by an earlier japa.chat call; omit to start a new conversation",
				},
				"tools": map[string]any{
					"type":        "object",
					"description": "Per-message tool selection; {enabled:false} runs a pure chat turn",
					"properties": map[string]any{
						"enabled": map[string]any{"type": "boolean"},
					},
				},
			},
			"required": []any{"message"},
		},
		Annotations: &toolAnnotations{
			Title:          "Japa Chat",
			ReadOnlyHint:   false, // a turn may write the transcript + memory
			IdempotentHint: false,
		},
	}
}

// handleJapaChat drains a unified engine turn and returns its result as a
// single text block: {reply, conversation_id, message_id, sources,
// session_id}. The audit session is created by the engine itself (§103).
func (s *Server) handleJapaChat(ctx context.Context, user *auth.AuthenticatedUser, req *jsonRPCRequest, callReq callToolRequest) (*jsonRPCResponse, bool) {
	if s.agent == nil {
		return s.errorResp(req.ID, -32602, "Invalid params", fmt.Sprintf("tool %q is not available", callReq.Name)), false
	}

	var args struct {
		Message        string `json:"message"`
		ConversationID string `json:"conversation_id"`
		Tools          *struct {
			Enabled *bool `json:"enabled"`
		} `json:"tools"`
	}
	if len(callReq.Arguments) > 0 && string(callReq.Arguments) != "null" {
		if err := json.Unmarshal(callReq.Arguments, &args); err != nil {
			return s.errorResp(req.ID, -32602, "Invalid params", err.Error()), false
		}
	}
	if strings.TrimSpace(args.Message) == "" {
		return s.errorResp(req.ID, -32602, "Invalid params", "message is required"), false
	}

	var convID *pgtype.UUID
	if args.ConversationID != "" {
		var u pgtype.UUID
		if err := u.Scan(args.ConversationID); err == nil && u.Valid {
			convID = &u
		}
		// Non-UUID ids (Honcho-era) are dropped — the engine opens a fresh
		// thread rather than erroring, matching POST /ai/chat.
	}

	var tools *ai.ToolSelection
	if args.Tools != nil {
		tools = &ai.ToolSelection{Enabled: args.Tools.Enabled}
	}

	events, err := s.agent.HandleUnifiedChat(ctx, user.UserID, ai.UnifiedChatRequest{
		Prompt:         args.Message,
		ConversationID: convID,
		Tools:          tools,
	})
	if err != nil {
		return s.errorResp(req.ID, -32602, "Invalid params", err.Error()), false
	}

	var reply, conversationID, messageID, sessionID string
	var sources []ai.RAGChunk
	var loopErr *ai.AgentEvent
	for ev := range events {
		switch ev.Type {
		case ai.EventFinalResponse:
			reply = ev.Content
			conversationID = ev.ConversationID
			messageID = ev.MessageID
		case ai.EventConversation:
			if ev.ConversationID != "" {
				conversationID = ev.ConversationID
			}
		case ai.EventSessionStart:
			sessionID = ev.SessionID
			if ev.ConversationID != "" {
				conversationID = ev.ConversationID
			}
		case ai.EventSources:
			sources = ev.Sources
		case ai.EventError:
			e := ev
			loopErr = &e
		}
		// delta / progress / tool_call_* events are intentionally skipped —
		// tools/call is unary.
	}

	if loopErr != nil {
		out, _ := json.Marshal(map[string]any{"error": loopErr.Error})
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: callToolResult{
				Content: []mcpContent{{Type: "text", Text: string(out)}},
				IsError: true,
			},
		}, false
	}

	out, _ := json.Marshal(map[string]any{
		"reply":           reply,
		"conversation_id": conversationID,
		"message_id":      messageID,
		"sources":         sources,
		"session_id":      sessionID,
	})
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: callToolResult{
			Content: []mcpContent{{Type: "text", Text: string(out)}},
		},
	}, false
}
