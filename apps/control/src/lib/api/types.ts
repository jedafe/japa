import type { Snippet } from 'svelte';

export interface User {
	id: string;
	username: string;
	display_name: string;
	email: string;
	avatar_url: string | null;
	user_group_id: string;
	user_group_name: string;
	theme_preference: 'light' | 'dark' | 'system';
	timezone: string;
	is_verified: boolean;
	created_at: string;
	permissions?: string[];
}

export interface HealthStatus {
	infrastructure: ServiceStatus[];
	application: ServiceStatus[];
	background: ServiceStatus[];
	realtime: ServiceStatus[];
}

export interface ServiceStatus {
	name: string;
	status: 'healthy' | 'degraded' | 'down';
	latency_ms: number;
	message?: string;
}

export interface QueueInfo {
	name: string;
	pending: number;
	active: number;
	completed_24h: number;
	failed_24h: number;
	latency_ms: number;
}

export interface QueueStats {
	queues: QueueInfo[];
	total_pending: number;
	total_active: number;
	total_completed_24h: number;
	total_failed_24h: number;
	dlq_count: number;
}

export interface QueueTask {
	id: string;
	type: string;
	state: string;
	payload: string;
	retry_count: number;
	max_retry: number;
	last_err?: string;
	last_failed_at?: string;
	next_process_at?: string;
}

export interface QueueTasksResponse {
	pending: QueueTask[];
	active: QueueTask[];
	scheduled: QueueTask[];
	retry: QueueTask[];
	archived: QueueTask[];
	completed: QueueTask[];
}

export interface DlqEntry {
	task_id: string;
	task_type: string;
	queue: string;
	error_message: string;
	failed_at: string;
	retry_count: number;
	max_retries: number;
	payload: Record<string, unknown>;
}

export interface CacheStats {
	namespaces: { prefix: string; key_count: number; memory_estimate_mb: number }[];
	total_keys: number;
	total_memory_mb: number;
}

export interface SystemSetting {
	key: string;
	value: string;
	type: 'int' | 'float' | 'bool' | 'string' | 'csv' | 'json';
	default_value: string;
	description: string;
	namespace: string;
	min?: number;
	max?: number;
	sensitive?: boolean;
	options?: { value: string; label: string }[];
}

export interface FeatureFlag {
	key: string;
	enabled: boolean;
	description: string;
	allowed_groups: string[];
}

export interface Tenant {
	id: string;
	name: string;
	slug: string;
	domain: string | null;
	status: string;
	created_at: string;
	updated_at: string;
}

export interface SecurityScan {
	last_scan_at: string;
	findings: SecurityFinding[];
	active_ip_blocks: IpBlock[];
	recent_suspicious_ips: string[];
}

export interface SecurityFinding {
	finding_type: string;
	severity: 'critical' | 'high' | 'medium' | 'low';
	description: string;
	affected_resource: string;
	recommendation: string;
}

export interface IpBlock {
	id: string;
	tenant_id: string;
	cidr: string;
	block_type: string;
	reason: string | null;
	blocked_by: string;
	expires_at: string | null;
	created_at: string;
}

export interface ApiKey {
	id: string;
	app_name: string;
	user: { id: string; username: string } | null;
	key_prefix: string;
	rate_limit_tier: string;
	status: string;
	request_count: number;
	last_used_at: string | null;
	scopes: string[];
	created_at: string;
}

export interface AiBudget {
	month_to_date_cost: number;
	budget_threshold_usd: number;
	alert_triggered: boolean;
	by_provider: { provider: string; tokens_used: number; cost_usd: number }[];
	by_task_type: { task_type: string; call_count: number; tokens_used: number; cost_usd: number }[];
}

export interface AuditLogEntry {
	id: string;
	timestamp: string;
	actor: { id: string; username: string } | null;
	action_type: string;
	entity_type: string;
	entity_id: string;
	tenant_id: string | null;
	ip_address: string;
	impersonated_user_id: string | null;
	metadata: Record<string, unknown>;
	user_agent: string;
}

export interface DbHealth {
	connected: boolean;
	version: string;
	latency_ms: number;
	size_bytes: number;
	table_count: number;
	index_count: number;
	avg_query_time_ms: number;
	tables: { name: string; live_rows: number; dead_rows: number; dead_ratio: number; row_count: number; size_bytes: number; index_count: number; last_vacuum: string | null }[];
	slow_queries: { pid: number; duration: number; duration_ms: number; state: string; query_preview: string; query: string; timestamp: string }[];
	indexes_unused: { name: string; table_name: string; scans: number }[];
	connection_pool?: { active: number; idle: number; max: number };
	replication?: { state: string; lag_seconds: number };
}

export interface Deployment {
	version: string;
	status: string;
	deployed_at: string;
}

export interface PaginatedResponse<T> {
	data: T[];
	total: number;
	page: number;
	per_page: number;
	total_pages: number;
}

export interface DataTableColumn<T = Record<string, unknown>> {
	key: string;
	label: string;
	sortable?: boolean;
	class?: string;
	render?: (value: unknown, row: T) => string;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type SvelteIcon = any;

export interface DataTableRowAction<T = Record<string, unknown>> {
	label: string;
	icon?: SvelteIcon;
	onClick: (row: T) => void;
	variant?: 'default' | 'destructive';
	disabled?: (row: T) => boolean;
	hidden?: (row: T) => boolean;
}

export interface DataTableBulkAction {
	label: string;
	icon?: SvelteIcon;
	onClick: (ids: string[]) => void;
	variant?: 'default' | 'destructive';
}
