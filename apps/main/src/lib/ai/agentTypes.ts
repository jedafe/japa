// Agent event + response types — mirror the Go structs in internal/ai/agent.go
// (AgentEvent) and internal/ai/agent_handlers.go (sessionToJSON, toolCallToJSON,
// toolStatsToJSON, costReportToJSON).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 6, Phase 10)

/** SSE event types emitted by the agent loop (internal/ai/agent.go constants). */
export type AgentEventType =
	| 'session_start'
	| 'tool_call_start'
	| 'tool_call_result'
	| 'progress'
	| 'final_response'
	| 'error';

/** Single SSE event from POST /api/v1/ai/agent. Fields are optional per event type. */
export interface AgentEvent {
	type: AgentEventType;
	session_id?: string;
	// session_start / progress
	presearch_level?: string;
	iteration?: number;
	message?: string;
	// tool_call_start / tool_call_result
	tool_name?: string;
	arguments?: string;
	status?: string;
	// tool_call_result / final_response / error
	content?: string;
	error?: string;
	// final_response
	tokens_used?: number;
	estimated_cost?: number;
}

/** Tool catalog entry from GET /api/v1/ai/agent/capabilities → data.tools[]. */
export interface AgentToolCapability {
	function_name: string;
	description: string;
	// JSON Schema for the tool's parameters (may be omitted in the UI).
	parameters?: Record<string, unknown>;
	requires_hitl?: boolean;
	required_permission?: string;
}

/** Geo context summary from GET /api/v1/ai/agent/capabilities → data.geo_context. */
export interface AgentGeoContext {
	city?: string;
	country?: string;
	country_code?: string;
	geo_space_id?: string;
}

/** Capabilities response (GET /api/v1/ai/agent/capabilities). */
export interface AgentCapabilities {
	tools: AgentToolCapability[];
	geo_context: AgentGeoContext | null;
}

/** Agent session row (sessionToJSON in agent_handlers.go). */
export interface AgentSession {
	id: string;
	user_id: string;
	prompt: string;
	presearch_level: string;
	status: string;
	tool_call_count: number;
	total_tokens: number;
	started_at: string;
	created_at: string;
	geo_space_id?: string;
	result_summary?: string;
	error_message?: string;
	completed_at?: string;
	estimated_cost: number;
}

/** Agent tool call row (toolCallToJSON in agent_handlers.go). */
export interface AgentToolCall {
	id: string;
	session_id: string;
	tool_name: string;
	status: string;
	rbac_checked: boolean;
	privacy_checked: boolean;
	created_at: string;
	arguments?: unknown;
	result?: unknown;
	error_message?: string;
	latency_ms?: number;
	hitl_review_id?: string;
}

/** Session detail response (GET /api/v1/ai/agent/sessions/{id}). */
export interface AgentSessionDetail {
	session: AgentSession;
	tool_calls: AgentToolCall[];
}

/** List-meta envelope for GET /api/v1/ai/agent/sessions. */
export interface AgentListMeta {
	total: number;
	page_size: number;
	page_offset: number;
}

/** Per-tool stats row (toolStatsToJSON in agent_handlers.go). */
export interface AgentToolStat {
	tool_name: string;
	total_calls: number;
	success_count: number;
	error_count: number;
	unauthorized_count: number;
	privacy_blocked_count: number;
	hitl_queued_count: number;
	success_rate: number;
	avg_latency_ms: number;
	max_latency_ms: number;
}

/** Cost report row (costReportToJSON in agent_handlers.go). */
export interface AgentCostReportRow {
	user_id: string;
	username?: string;
	session_count: number;
	total_tokens: number;
	total_tool_calls: number;
	total_cost_usd: number;
}

/** Privacy exemption status (GET /api/v1/me/ai-privacy → data). */
export interface AIPrivacyStatus {
	profile_ai_exempt: boolean;
}

/** Privacy exemption update result (PUT /api/v1/me/ai-privacy → data). */
export interface AIPrivacyUpdateResult {
	entity_type: string;
	entity_id: string;
	ai_exempt: boolean;
}
