// Agent event + response types — mirror the Go structs in internal/ai/agent.go
// (AgentEvent) and internal/ai/agent_handlers.go (sessionToJSON, toolCallToJSON,
// toolStatsToJSON, costReportToJSON).
//
// §115: stream/event/conversation types live in @japaconnect/ai-panel and are
// re-exported here so existing imports keep working (AgentRequestError must be
// a VALUE re-export — JapaAIChat uses `instanceof`).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 6, Phase 10)

import type { ChatTemplate } from './chatTemplate';
import type { ToolSelectionOverride } from './unifiedStream';

export { AgentRequestError } from '@japaconnect/ai-panel';
export type {
	AgentEventType,
	AgentEvent,
	AgentToolCapability,
	AgentGeoContext,
	AgentToolCategory,
	AgentCapabilities,
	UnifiedConversationSummary,
	ConversationToolCall,
	ConversationMessage
} from '@japaconnect/ai-panel';

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

/** BYOK provider storefront row from GET /ai/byok/providers. */
export interface ByokProviderOption {
	id: string;
	provider_type: string;
	display_name: string;
	models: { id: string; label: string }[];
	signup_url: string;
	signup_label: string;
	has_key: boolean;
	keys: { id: string; label: string; api_key_masked: string; status: string }[];
}

/** Caller-owned BYOK credential from /me/ai-keys (masked only). */
export interface UserAIKeyView {
	id: string;
	byok_provider_id: string;
	label: string;
	api_key_masked: string;
	status: string;
	last_error?: string;
	created_at: string;
	provider: ByokProviderOption;
}
