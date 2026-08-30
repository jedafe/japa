import type { ListMeta } from './index';

export interface ComparisonTypeConfig {
	max_item_count?: number;
	default_item_count?: number;
	item_ref_types?: string[];
	default_item_ref_type?: string;
	allow_nested_comparisons?: boolean;
	allowed_nested_types?: string[];
	[k: string]: unknown;
}

export interface ComparisonType {
	id: string;
	name: string;
	slug: string;
	description?: string | null;
	icon?: string | null;
	item_label: string;
	item_count?: number;
	field_count?: number;
	config?: ComparisonTypeConfig | null;
}

export interface ResolvedItem {
	type: string;
	id: string;
	title: string;
	description?: string;
	image_url?: string;
	url?: string;
	metadata?: Record<string, unknown>;
}

export interface JCEItem {
	id?: string;
	title: string;
	description?: string;
	type_id?: string;
	position?: number;
	item_ref?: Record<string, unknown> | null;
}

export interface JCEDimension {
	id: string;
	content_id?: string;
	label: string;
	dimension_type?: string;
	position?: number;
}

export interface JCEComparisonContent {
	id: string;
	title: string;
	slug: string;
	body?: string;
	body_html?: string;
	space_id?: string;
	content_type?: string;
	status: 'draft' | 'published' | 'unlisted' | 'private';
	visibility: 'public' | 'unlisted' | 'private';
	author_id?: string;
	created_at?: string;
	updated_at?: string;
}

export interface JCEComparisonMetadata {
	content_id: string;
	depth: 'quick' | 'detailed';
	promoted_from_id?: string | null;
	promoted_at?: string | null;
	smart_mode?: string;
	config?: Record<string, unknown> | null;
}

export interface JCEComparisonDetail {
	content: JCEComparisonContent;
	metadata: JCEComparisonMetadata;
	items: JCEItem[];
	dimensions: JCEDimension[];
	type_slug?: string;
}

export interface JCEComparisonSummary {
	id: string;
	title: string;
	slug: string;
	body?: string;
	body_html?: string;
	excerpt?: string;
	status: string;
	visibility: string;
	depth: 'quick' | 'detailed';
	author_id?: string;
	space_id?: string;
	type_id?: string;
	type_slug?: string;
	type_name?: string;
	view_count?: number;
	comment_count?: number;
	share_count?: number;
	created_at?: string;
	updated_at?: string;
}

export interface MatrixItem {
	id: string;
	title: string;
	description?: string;
	image_url?: string;
	position?: number;
	dimensions: MatrixDimension[];
}

export interface MatrixDimension {
	dimension_id: string;
	label: string;
	type: string;
	value?: string | null;
	value_rating?: number | null;
	value_boolean?: boolean | null;
}

export interface SelectionToken {
	token: string;
}

export interface CreateQuickComparisonInput {
	title: string;
	space_id?: string;
	type_id: string;
	depth: 'quick';
	status: 'draft';
	visibility: 'private';
	items: JCEItem[];
	dimensions: string[];
	smart_mode?: string;
	config?: Record<string, unknown>;
}

export interface ComparisonProfile {
	id: string;
	title: string;
	source_space_id?: string | null;
	target_space_ids: string[];
	goals: { key: string; weight: number }[];
	preferences: Record<string, number>;
	demographics: {
		age?: number;
		occupation?: string;
		family_status?: string;
		language_proficiency?: string;
		budget_min?: number;
		budget_max?: number;
		budget_currency?: string;
	};
	is_default: boolean;
	is_template: boolean;
	created_at?: string;
	updated_at?: string;
}

export interface ScoreFactor {
	id: string;
	comparison_type_id?: string | null;
	factor_key: string;
	label: string;
	description?: string;
	dimension_slug?: string | null;
	data_source: 'dimension' | 'resolver_metadata' | 'profile' | 'eligibility' | 'aggregate';
	default_weight: number;
	config: Record<string, any>;
	version: number;
	is_system: boolean;
}

export interface FactorScore {
	factor_key: string;
	label: string;
	score_value: number;
	max_value: number;
	weight: number;
	explanation: string;
	missing?: boolean;
	missing_score?: number;
}

export interface JapaScore {
	item_id: string;
	item_title: string;
	japa_score: number;
	max_value: number;
	eligible_status: 'eligible' | 'limited' | 'ineligible';
	factor_scores: FactorScore[];
	explanation: string;
}

export interface ComparisonRecommendation {
	id: string;
	item_id?: string | null;
	text: string;
	action_type?: string | null;
	action_url?: string | null;
	priority: number;
}

export interface ScoreResult {
	content_id: string;
	profile_id?: string | null;
	smart_mode: string;
	scores: JapaScore[];
	recommendations: ComparisonRecommendation[];
	weights_snapshot: Record<string, number>;
	recommendation_summary: {
		top_item_id: string;
		top_japa_score: number;
		recommendations_count: number;
		last_scored_at: string;
	};
	version: number;
}

export interface ScorePreviewInput {
	type_id: string;
	smart_mode: string;
	item_refs: Record<string, unknown>[];
	profile?: ComparisonProfileInput;
}

export interface ComparisonProfileInput {
	title?: string;
	source_space_id?: string | null;
	target_space_ids: string[];
	goals?: { key: string; weight: number }[];
	preferences?: Record<string, number>;
	demographics?: ComparisonProfile['demographics'];
	is_default?: boolean;
	is_template?: boolean;
}
