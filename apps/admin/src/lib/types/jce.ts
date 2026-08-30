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
