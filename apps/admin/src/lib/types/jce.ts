export interface DataSource {
	id: string;
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
