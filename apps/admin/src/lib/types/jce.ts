export interface ScoreFactorAdmin {
	id: string;
	tenant_id?: string;
	comparison_type_id?: string | null;
	factor_key: string;
	label: string;
	description?: string;
	dimension_slug?: string | null;
	data_source: string;
	default_weight: number;
	config: Record<string, any>;
	version: number;
	is_system: boolean;
	created_at?: string;
	updated_at?: string;
}

export interface DataSource {
	id: string;
	tenant_id: string;
	entity_type?: string | null;
	entity_id?: string | null;
	dimension_slug?: string | null;
	source_name: string;
	source_url?: string | null;
	source_kind: 'official' | 'third_party' | 'ai_estimate';
	trust_level: number;
	freshness_score: number;
	last_verified_at?: string | null;
	config: Record<string, unknown>;
	created_at: string;
	updated_at: string;
}

export type DataSourceAdmin = DataSource;

export interface IngestionJob {
	id: string;
	tenant_id: string;
	content_id: string;
	status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
	source_kind?: string | null;
	refreshed_count: number;
	error?: string | null;
	started_at?: string | null;
	completed_at?: string | null;
	created_at: string;
	updated_at: string;
}

export type IngestionJobAdmin = IngestionJob;
