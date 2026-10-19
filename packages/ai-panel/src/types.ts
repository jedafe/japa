// Shared types for the §115 AI Quick Panel — mirrors the Go structs in
// internal/ai/agent.go (AgentEvent) and internal/ai/agent_handlers.go.
// Ref: japaconnect-engineering/sections/115-ai-quick-panel.md
// Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md

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
	| 'delta'
	| 'final_response'
	| 'error'
	| 'conversation'
	| 'sources';

/** RAG citation source emitted by /ai/chat (§69 Citation Source Contract). */
export interface AiCitationSource {
	entity_type?: string;
	entity_id?: string;
	similarity?: number;
	tier?: string;
	title?: string;
	slug?: string;
	url?: string;
}

/** Single SSE event from POST /api/v1/ai/chat. Fields are optional per event type. */
export interface AgentEvent {
	type: AgentEventType;
	session_id?: string;
	presearch_level?: string;
	iteration?: number;
	message?: string;
	tool_name?: string;
	arguments?: string;
	status?: string;
	content?: string;
	error?: string;
	tokens_used?: number;
	estimated_cost?: number;
	conversation_id?: string;
	project_id?: string;
	message_id?: string;
	turn_count?: number;
	sources?: AiCitationSource[];
}

/** Tool catalog entry from GET /api/v1/ai/agent/capabilities → data.tools[]. */
export interface AgentToolCapability {
	function_name: string;
	description: string;
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
	categories?: AgentToolCategory[];
	geo_context: AgentGeoContext | null;
}

/** Unified conversation summary (GET /api/v1/ai/conversations → data[]). */
export interface UnifiedConversationSummary {
	conversation_id: string;
	title?: string;
	preview?: string;
	turn_count: number;
	last_entry_at?: string;
	legacy: boolean;
	project_id?: string;
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

/** Per-message tool override — mirrors the Go ToolSelection (internal/ai/prefs.go). */
export interface ToolSelectionOverride {
	enabled?: boolean;
	categories?: string[];
	tools?: string[];
}

/** Options for the streamChat generator. */
export interface StreamChatOptions {
	message: string;
	conversationId?: string;
	projectId?: string;
	/** §115 page anchor — find-or-create the page's thread server-side. */
	anchor?: string;
	model?: string;
	/** BYOK — routes the turn through the caller's own provider key. */
	byokKeyId?: string;
	voiceId?: string;
	context?: Record<string, unknown>;
	humanize?: boolean;
	humanizeLevel?: string;
	tools?: ToolSelectionOverride;
	signal?: AbortSignal;
}

/** Host-app injected dependencies — the package can't reach app $lib modules. */
export interface AiPanelDeps {
	apiBase: string;
	surface: string;
	getCsrfToken: () => string | null;
	refreshCsrfToken: () => Promise<unknown>;
	t?: (key: string, fallback?: string, params?: Record<string, string | number>) => string;
	fullChatUrl: (conversationId?: string) => string;
}

/** Page context published by the host app's route extractor. */
export interface AiPageContext {
	anchor: string;
	label: string;
	context: Record<string, unknown>;
}
