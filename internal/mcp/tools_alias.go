// Package mcp — tools_alias.go implements the ChatGPT deep-research tool
// contract: two tools named exactly "search" and "fetch" (OpenAI's connector
// evaluator requires these names). They are thin aliases over content.search
// and content.read — no ai_tools rows, no registry entries — reshaping the
// executor's JSON into OpenAI's {id,title,url} / {id,title,text,url} shape.
//
// Gating inherits the underlying tools' api.read.content scope (§120).
// Audit sessions record "mcp:search"/"mcp:fetch"; ai_agent_tool_calls keeps
// the real tool name so executor-level RBAC/privacy checks are untouched.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
)

// chatGPTAliasTool binds an alias name to its underlying catalog tool.
type chatGPTAliasTool struct {
	underlying  string
	description string
	schema      map[string]any
}

var chatGPTAliasTools = map[string]chatGPTAliasTool{
	"search": {
		underlying:  "content.search",
		description: "Search published JapaConnect content — posts, articles, guides, visa and relocation resources. Returns id, title, and url for each result; call fetch with an id to read the full text.",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
			},
			"required": []any{"query"},
		},
	},
	"fetch": {
		underlying:  "content.read",
		description: "Fetch the full text of a JapaConnect content item by the id returned from search.",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "Content id from a search result"},
			},
			"required": []any{"id"},
		},
	},
}

// aliasDescriptors returns the MCP tool descriptors for the search/fetch
// aliases the caller is allowed to see — gated like the underlying tool and
// only offered when the real tool is in the caller's catalog. Sorted for a
// deterministic tools/list payload.
func (s *Server) aliasDescriptors(ctx context.Context, user *auth.AuthenticatedUser, tier string) []mcpTool {
	names := make([]string, 0, len(chatGPTAliasTools))
	for name := range chatGPTAliasTools {
		names = append(names, name)
	}

	sort.Strings(names)

	var out []mcpTool

	for _, name := range names {
		alias := chatGPTAliasTools[name]

		if s.toolDeniedWithTier(ctx, user, name, tier) != "" {
			continue
		}

		if !s.toolInCatalog(ctx, user, alias.underlying) {
			continue
		}

		title := humanizeToolName(name)
		out = append(out, mcpTool{
			Name:        name,
			Title:       title,
			Description: alias.description,
			InputSchema: alias.schema,
			Annotations: &toolAnnotations{
				Title:          title,
				ReadOnlyHint:   true,
				IdempotentHint: true,
			},
		})
	}

	return out
}

// handleAliasCall executes a search/fetch alias: gate check → catalog check
// on the underlying tool → session → executor → contract-shaped result. The
// catalog check mirrors tools/list so a caller cannot invoke an alias for a
// tool that isn't in their catalog.
func (s *Server) handleAliasCall(ctx context.Context, user *auth.AuthenticatedUser, req *jsonRPCRequest, callReq callToolRequest, alias chatGPTAliasTool) (*jsonRPCResponse, bool) {
	if !s.toolInCatalog(ctx, user, alias.underlying) {
		return s.errorResp(req.ID, -32602, "Invalid params", fmt.Sprintf("tool %q is not available", callReq.Name)), false
	}

	var args map[string]any
	if err := json.Unmarshal(callReq.Arguments, &args); err != nil {
		return s.errorResp(req.ID, -32602, "Invalid params", err.Error()), false
	}

	var mapped []byte

	var err error

	switch callReq.Name {
	case "search":
		q, _ := args["query"].(string)
		if strings.TrimSpace(q) == "" {
			return s.errorResp(req.ID, -32602, "Invalid params", "query is required"), false
		}

		mapped, err = json.Marshal(map[string]any{"query": q, "limit": 10})
	case "fetch":
		id, _ := args["id"].(string)
		if strings.TrimSpace(id) == "" {
			return s.errorResp(req.ID, -32602, "Invalid params", "id is required"), false
		}

		mapped, err = json.Marshal(map[string]any{"content_id": id})
	default:
		return s.errorResp(req.ID, -32602, "Invalid params", fmt.Sprintf("tool %q is not available", callReq.Name)), false
	}

	if err != nil {
		return s.errorResp(req.ID, -32602, "Invalid params", err.Error()), false
	}

	session, serr := s.sessions.CreateAgentSession(ctx, gen.CreateAgentSessionParams{
		UserID:         user.UserID,
		Prompt:         "mcp:" + callReq.Name,
		PresearchLevel: "none",
		Status:         "running",
	})
	if serr != nil {
		return s.errorResp(req.ID, -32603, "Internal error", "failed to create audit session"), false
	}

	call := ai.ToolCall{ID: uuid.NewString(), Name: alias.underlying, Arguments: mapped}
	res, execErr := s.executor.Execute(ctx, ai.ExecuteParams{Call: call, SessionID: session.ID, UserID: user.UserID})

	s.finalizeSession(ctx, session.ID, callReq.Name, res, execErr)

	text := ""
	isError := true

	if res != nil {
		text = res.Content
		isError = execErr != nil || (res.Status != ai.ToolCallStatusSuccess && res.Status != ai.ToolCallStatusHITLQueued)
	}

	if text == "" {
		text = `{"error":"tool execution failed"}`
	}

	if !isError {
		text = s.reshapeAliasResult(callReq.Name, text)
	}

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: callToolResult{
			Content: []mcpContent{{Type: "text", Text: text}},
			IsError: isError,
		},
	}, false
}

// reshapeAliasResult converts the underlying tool's JSON into the shape the
// deep-research contract expects. On any parse problem the raw content is
// returned unchanged — the caller still gets usable data.
func (s *Server) reshapeAliasResult(name, content string) string {
	switch name {
	case "search":
		var payload struct {
			Results []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
				Slug  string `json:"slug"`
			} `json:"results"`
		}

		if err := json.Unmarshal([]byte(content), &payload); err != nil {
			return content
		}

		type item struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			URL   string `json:"url"`
		}

		out := make([]item, 0, len(payload.Results))
		for _, r := range payload.Results {
			out = append(out, item{ID: r.ID, Title: r.Title, URL: s.contentURL(r.ID, r.Slug)})
		}

		b, err := json.Marshal(map[string]any{"results": out})
		if err != nil {
			return content
		}

		return string(b)

	case "fetch":
		var payload struct {
			Content struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				Slug        string `json:"slug"`
				Body        string `json:"body"`
				ContentType string `json:"content_type"`
				CreatedAt   string `json:"created_at"`
				AuthorID    string `json:"author_id"`
			} `json:"content"`
		}

		if err := json.Unmarshal([]byte(content), &payload); err != nil {
			return content
		}

		b, err := json.Marshal(map[string]any{
			"id":    payload.Content.ID,
			"title": payload.Content.Title,
			"text":  payload.Content.Body,
			"url":   s.contentURL(payload.Content.ID, payload.Content.Slug),
			"metadata": map[string]any{
				"content_type": payload.Content.ContentType,
				"created_at":   payload.Content.CreatedAt,
				"author":       payload.Content.AuthorID,
			},
		})
		if err != nil {
			return content
		}

		return string(b)
	}

	return content
}

// contentURL builds the canonical page URL for a content result. The public
// content route is {SiteURL}/content/{slug}; without a slug the API URL is
// used as a resolvable fallback identifier.
func (s *Server) contentURL(id, slug string) string {
	base := strings.TrimRight(s.siteBaseURL, "/")
	if slug != "" {
		return base + "/content/" + slug
	}

	return base + "/api/v1/public/content/" + id
}
