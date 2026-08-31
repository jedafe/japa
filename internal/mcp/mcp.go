// Package mcp implements the JapaConnect Model Context Protocol (MCP) bridge.
//
// Ref: japaconnect-engineering/sections/119-mcp-server.md
// Ref: japaconnect-engineering/adr/ADR-116-mcp-server.md
//
// The bridge is a deterministic, LLM-free tool surface. Clients send JSON-RPC
// 2.0 requests to POST /api/v1/mcp or open an SSE stream via GET /api/v1/mcp;
// the server maps tools/list and tools/call onto the existing ai.Executor
// (Section 103), preserving RBAC, privacy, HITL, and the
// ai_agent_sessions/ai_agent_tool_calls audit trail.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
)

const (
	protocolVersion = "2024-11-05"
	serverName      = "japaconnect-mcp"
	serverVersion   = "1.0.0"
	maxBodyBytes    = 1 << 20 // 1 MiB
)

// supportedClientVersions lists protocol versions the v1 bridge can accept.
// The server always advertises protocolVersion as the negotiated version.
var supportedClientVersions = []string{protocolVersion, "2025-03-26", "2025-11-25"}

func supportsProtocolVersion(v string) bool {
	for _, sv := range supportedClientVersions {
		if sv == v {
			return true
		}
	}
	return false
}

// SessionStore is the subset of gen.Queries the MCP server needs to create
// and close the per-tool-call audit session.
type SessionStore interface {
	CreateAgentSession(ctx context.Context, arg gen.CreateAgentSessionParams) (gen.AiAgentSession, error)
	CompleteAgentSession(ctx context.Context, arg gen.CompleteAgentSessionParams) (gen.AiAgentSession, error)
	FailAgentSession(ctx context.Context, arg gen.FailAgentSessionParams) (gen.AiAgentSession, error)
}

// Server is an http.Handler that speaks MCP over a single HTTP endpoint.
type Server struct {
	executor *ai.Executor
	registry *ai.ToolHandlerRegistry
	sessions SessionStore
	perms    ai.PermissionChecker
}

// NewServer creates an MCP server handler backed by the agent executor.
func NewServer(executor *ai.Executor, registry *ai.ToolHandlerRegistry, sessions SessionStore, perms ai.PermissionChecker) *Server {
	return &Server{
		executor: executor,
		registry: registry,
		sessions: sessions,
		perms:    perms,
	}
}

// jsonRPCRequest is the envelope for every MCP message.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// jsonRPCResponse is the envelope for every MCP reply.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ServeHTTP implements http.Handler. It accepts both POST (JSON-RPC request/
// response) and GET (Streamable HTTP / legacy SSE stream). All other methods
// return a JSON-RPC error.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil || !user.UserID.Valid {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		s.writeError(w, nil, -32001, "Unauthorized", "authentication required")
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handlePost(w, r, user)
	case http.MethodGet:
		s.handleGet(w, r, user)
	default:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		s.writeError(w, nil, -32600, "Invalid Request", "only POST and GET are supported")
	}
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request, user *auth.AuthenticatedUser) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		s.writeError(w, nil, -32700, "Parse error", err.Error())
		return
	}

	var req jsonRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeError(w, nil, -32700, "Parse error", err.Error())
		return
	}

	if req.JSONRPC != "2.0" {
		s.writeError(w, req.ID, -32600, "Invalid Request", "jsonrpc must be 2.0")
		return
	}

	resp, isNotification := s.handle(r.Context(), user, &req)
	if isNotification {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if resp == nil {
		resp = &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &jsonRPCError{Code: -32603, Message: "Internal error"}}
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleGet opens a server-to-client SSE stream for Streamable HTTP and legacy
// SSE transports. The v1 implementation is stateless, so the endpoint event
// points back to the same URL and the stream stays open until the client
// disconnects.
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, user *auth.AuthenticatedUser) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sessionID := uuid.NewString()
	postURL := r.URL.Path + "?session_id=" + sessionID

	flusher, ok := w.(http.Flusher)
	if !ok {
		slog.Warn("mcp: response writer does not support flushing, SSE will not stream incrementally")
	}

	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", postURL)
	if flusher != nil {
		flusher.Flush()
	}

	// Block until the client disconnects or the request context is cancelled.
	<-r.Context().Done()
}

func (s *Server) handle(ctx context.Context, user *auth.AuthenticatedUser, req *jsonRPCRequest) (*jsonRPCResponse, bool) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(ctx, req)
	case "notifications/initialized":
		return nil, true
	case "tools/list":
		return s.handleToolsList(ctx, user, req)
	case "tools/call":
		return s.handleToolsCall(ctx, user, req)
	default:
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32601, Message: "Method not found"},
		}, false
	}
}

// ---------------------------------------------------------------------------
// initialize
// ---------------------------------------------------------------------------

type initializeRequest struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      serverInfo     `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (s *Server) handleInitialize(ctx context.Context, req *jsonRPCRequest) (*jsonRPCResponse, bool) {
	var initReq initializeRequest
	if err := json.Unmarshal(req.Params, &initReq); err != nil {
		return s.errorResp(req.ID, -32602, "Invalid params", err.Error()), false
	}
	if initReq.ProtocolVersion != "" && !supportsProtocolVersion(initReq.ProtocolVersion) {
		return s.errorResp(req.ID, -32602, "Unsupported protocol version", initReq.ProtocolVersion), false
	}

	result := initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities: map[string]any{
			"tools": map[string]any{
				"listChanged": false,
			},
		},
		ServerInfo: serverInfo{
			Name:    serverName,
			Version: serverVersion,
		},
	}
	return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}, false
}

// ---------------------------------------------------------------------------
// tools/list
// ---------------------------------------------------------------------------

type listToolsRequest struct {
	Cursor string `json:"cursor"`
}

type listToolsResult struct {
	Tools      []mcpTool `json:"tools"`
	NextCursor string    `json:"nextCursor,omitempty"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

func (s *Server) handleToolsList(ctx context.Context, user *auth.AuthenticatedUser, req *jsonRPCRequest) (*jsonRPCResponse, bool) {
	var listReq listToolsRequest
	_ = json.Unmarshal(req.Params, &listReq) // cursor is optional

	toolConfigs := s.registry.CatalogForUser(ctx, s.perms, user.UserID, user.TenantID)
	tools := make([]mcpTool, 0, len(toolConfigs))
	for _, tc := range toolConfigs {
		schema := tc.Parameters
		if schema == nil {
			schema = map[string]any{"type": "object"}
		} else {
			schema = shallowCopyMap(schema)
			if _, ok := schema["type"]; !ok {
				schema["type"] = "object"
			}
		}
		tools = append(tools, mcpTool{
			Name:        tc.FunctionName,
			Description: tc.Description,
			InputSchema: schema,
		})
	}

	result := listToolsResult{Tools: tools}
	return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}, false
}

// ---------------------------------------------------------------------------
// tools/call
// ---------------------------------------------------------------------------

type callToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type callToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *Server) handleToolsCall(ctx context.Context, user *auth.AuthenticatedUser, req *jsonRPCRequest) (*jsonRPCResponse, bool) {
	var callReq callToolRequest
	if err := json.Unmarshal(req.Params, &callReq); err != nil {
		return s.errorResp(req.ID, -32602, "Invalid params", err.Error()), false
	}
	if callReq.Name == "" {
		return s.errorResp(req.ID, -32602, "Invalid params", "tool name is required"), false
	}

	// Validate that the tool is in the user's catalog (fail-fast with a clear
	// method-level error instead of executor's generic unauthorized).
	if !s.toolInCatalog(ctx, user, callReq.Name) {
		return s.errorResp(req.ID, -32602, "Invalid params", fmt.Sprintf("tool %q is not available", callReq.Name)), false
	}

	args := callReq.Arguments
	if len(args) == 0 || string(args) == "null" {
		args = []byte("{}")
	}

	// Create an audit session for the tool call.
	session, err := s.sessions.CreateAgentSession(ctx, gen.CreateAgentSessionParams{
		TenantID:       user.TenantID,
		UserID:         user.UserID,
		Prompt:         fmt.Sprintf("mcp:%s", callReq.Name),
		PresearchLevel: "none",
		Status:         "running",
	})
	if err != nil {
		slog.ErrorContext(ctx, "mcp: create audit session", "error", err)
		return s.errorResp(req.ID, -32603, "Internal error", "failed to create audit session"), false
	}

	call := ai.ToolCall{
		ID:        uuid.NewString(),
		Name:      callReq.Name,
		Arguments: args,
	}

	res, execErr := s.executor.Execute(ctx, ai.ExecuteParams{
		Call:      call,
		SessionID: session.ID,
		UserID:    user.UserID,
		TenantID:  user.TenantID,
	})

	s.finalizeSession(ctx, session.ID, user.TenantID, callReq.Name, res, execErr)

	result := s.buildCallResult(res, execErr)
	return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}, false
}

func (s *Server) toolInCatalog(ctx context.Context, user *auth.AuthenticatedUser, name string) bool {
	tools := s.registry.CatalogForUser(ctx, s.perms, user.UserID, user.TenantID)
	for _, tc := range tools {
		if tc.FunctionName == name {
			return true
		}
	}
	return false
}

func (s *Server) buildCallResult(res *ai.ToolResult, execErr error) callToolResult {
	if res == nil {
		return callToolResult{
			Content: []mcpContent{{Type: "text", Text: `{"error":"tool execution failed"}`}},
			IsError: true,
		}
	}

	text := res.Content
	if text == "" {
		text = "{}"
	}

	isError := execErr != nil ||
		(res.Status != ai.ToolCallStatusSuccess && res.Status != ai.ToolCallStatusHITLQueued)

	if res.Status == ai.ToolCallStatusHITLQueued {
		if reviewID, ok := hitlReviewID(res); ok {
			text = withHITLReview(text, reviewID)
		}
	}

	return callToolResult{
		Content: []mcpContent{{Type: "text", Text: text}},
		IsError: isError,
	}
}

func hitlReviewID(res *ai.ToolResult) (string, bool) {
	if res == nil || res.Metadata == nil {
		return "", false
	}
	v, ok := res.Metadata["hitl_review_id"]
	if !ok {
		return "", false
	}
	switch id := v.(type) {
	case pgtype.UUID:
		if id.Valid {
			return id.String(), true
		}
	case string:
		return id, true
	}
	return "", false
}

func withHITLReview(content, reviewID string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		payload = map[string]any{"result": content}
	}
	payload["hitl_review_id"] = reviewID
	payload["status"] = "hitl_queued"
	b, err := json.Marshal(payload)
	if err != nil {
		return content
	}
	return string(b)
}

func (s *Server) finalizeSession(ctx context.Context, sessionID, tenantID pgtype.UUID, toolName string, res *ai.ToolResult, execErr error) {
	if execErr != nil {
		msg := execErr.Error()
		if _, err := s.sessions.FailAgentSession(ctx, gen.FailAgentSessionParams{
			ErrorMessage:  &msg,
			ToolCallCount: 1,
			TotalTokens:   0,
			EstimatedCost: zeroNumeric(),
			ID:            sessionID,
			TenantID:      tenantID,
		}); err != nil {
			slog.ErrorContext(ctx, "mcp: fail agent session", "error", err)
		}
		return
	}

	status := ai.ToolCallStatusSuccess
	if res != nil && res.Status != "" {
		status = res.Status
	}
	summary := fmt.Sprintf("mcp %s: %s", toolName, status)
	if _, err := s.sessions.CompleteAgentSession(ctx, gen.CompleteAgentSessionParams{
		ResultSummary: &summary,
		ToolCallCount: 1,
		TotalTokens:   0,
		EstimatedCost: zeroNumeric(),
		ID:            sessionID,
		TenantID:      tenantID,
	}); err != nil {
		slog.ErrorContext(ctx, "mcp: complete agent session", "error", err)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func (s *Server) errorResp(id json.RawMessage, code int, message string, data any) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonRPCError{Code: code, Message: message, Data: data},
	}
}

func (s *Server) writeError(w http.ResponseWriter, id json.RawMessage, code int, message, data string) {
	s.writeJSON(w, http.StatusOK, s.errorResp(id, code, message, data))
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, resp *jsonRPCResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("mcp: write json response", "error", err)
	}
}

func shallowCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func zeroNumeric() pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
}
