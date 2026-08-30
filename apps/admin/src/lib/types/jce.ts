export interface ScoreFactorAdmin {
	id: string;
	comparison_type_id?: string | null;
	factor_key: string;
	label: string;
	description?: string;
	dimension_slug?: string | null;
	data_source: string;
	default_weight: number;
	config: Record<string, any>;
	is_system: boolean;
}
