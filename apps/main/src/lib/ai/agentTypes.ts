// Agent event + response types — mirror the Go structs in internal/ai/agent.go
// (AgentEvent) and internal/ai/agent_handlers.go (sessionToJSON, toolCallToJSON,
// toolStatsToJSON, costReportToJSON).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 6, Phase 10)

import type { AiCitationSource } from './types';
import type { ChatTemplate } from './chatTemplate';
import type { ToolSelectionOverride } from './unifiedStream';

/** Error thrown when an AI chat request fails pre-checks (non-SSE response
 * with a JSON error body). */
export class AgentRequestError extends Error {
	constructor(
		public readonly status: number,
		public readonly code: string,
		message: string
	) {
		super(message);
		this.name = 'AgentRequestError';
	}
}

/** SSE event types emitted by the agent loop (internal/ai/agent.go constants). */
export type AgentEventType =
	| 'session_start'
	| 'tool_call_start'
	| 'tool_call_result'
	| 'progress'
	// ADR-130 Phase 2: token-level text chunk — {content} holds a fragment;
	// accumulate to render the in-flight reply before final_response.
	| 'delta'
	| 'final_response'
	| 'error'
	// ADR-130 unified surface additions
	| 'conversation'
	| 'sources';

/** Single SSE event from POST /api/v1/ai/agent or /api/v1/ai/chat. Fields are optional per event type. */
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
	// ADR-130: conversation + final_response
	conversation_id?: string;
	// §103 AI Projects: owning project id on conversation events.
	project_id?: string;
	message_id?: string;
	// ADR-130: server-authoritative persisted message count on
	// conversation/final_response — sidebar assigns, never increments.
	turn_count?: number;
	// ADR-130: sources event — RAG citations for the turn
	sources?: AiCitationSource[];
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

/** A tool category group (ADR-130) — drives the composer's category chips. */
export interface AgentToolCategory {
	name: string;
	tools: { name: string; description: string }[];
}

/** Capabilities response (GET /api/v1/ai/agent/capabilities). */
export interface AgentCapabilities {
	tools: AgentToolCapability[];
	/** ADR-130: the same catalog grouped by ai_tools.category. */
	categories?: AgentToolCategory[];
	geo_context: AgentGeoContext | null;
}

/** Per-user AI preferences (GET/PUT /api/v1/me/ai-preferences → data). */
export interface AIPrefs {
	default_mode?: '' | 'chat' | 'agent';
	disabled_categories?: string[];
	disabled_tools?: string[];
	enabled_tools?: string[];
	/** Saved composer generation-spec presets (/ai chat templates). Values
	 * is opaque to the backend — see ./chatTemplate.ts for the shape. */
	chat_templates?: ChatTemplate[];
}

/** Unified conversation summary (GET /api/v1/ai/conversations → data[]). */
export interface UnifiedConversationSummary {
	conversation_id: string;
	title?: string;
	preview?: string;
	turn_count: number;
	last_entry_at?: string;
	/** true = Honcho-era thread — read-only, not deletable. */
	legacy: boolean;
	/** §103 AI Projects — owning project's id; absent when ungrouped. */
	project_id?: string;
}

/** AI project row (GET /api/v1/ai/projects → data[] or single). */
export interface AiProject {
	id: string;
	name: string;
	instructions: string;
	default_model: string;
	default_tone: string;
	default_voice_id: string;
	default_tools: ToolSelectionOverride | Record<string, unknown>;
	conversation_count?: number;
	session_count?: number;
	created_at?: string;
	updated_at?: string;
}

/** Tool call embedded on a conversation-detail assistant message. */
export interface ConversationToolCall {
	id: string;
	session_id: string;
	tool_name: string;
	status: string;
	rbac_checked?: boolean;
	privacy_checked?: boolean;
	created_at?: string;
	arguments?: unknown;
	result?: unknown;
	error_message?: string;
	latency_ms?: number;
	hitl_review_id?: string;
}

/** A message row in GET /api/v1/ai/conversations/{id} (unified threads). */
export interface ConversationMessage {
	id: string;
	conversation_id: string;
	role: 'user' | 'assistant';
	content: string;
	model?: string | null;
	voice_id?: string | null;
	tokens_used?: number;
	cost_usd?: string | number;
	agent_session_id?: string;
	created_at: string;
	tool_calls?: ConversationToolCall[];
}

/** Agent session row (sessionToJSON in agent_handlers.go). */
export interface AgentSession {
	id: string;
	user_id: string;
	/** §103 AI Projects — owning project's id; absent when ungrouped. */
	project_id?: string;
	/** §103 — linked conversation id; such sessions mirror the thread's project. */
	conversation_id?: string;
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
