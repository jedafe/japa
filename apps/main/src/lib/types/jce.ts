import type { ListMeta } from './index';

export interface ComparisonTypeConfig {
	max_item_count?: number;
	default_item_count?: number;
	item_ref_types?: string[];
	default_item_ref_type?: string;
	allow_nested_comparisons?: boolean;
	allowed_nested_types?: string[];
	scope_chain?: string[];
	content_type?: string;
	/** Selector grouping (JCE ↔ wiki mapping): family card the type belongs to. */
	family?: string;
	/** Host wiki-type level within the family (country | state | city | …). */
	level?: string;
	/** Wiki subpage subtype within a family (e.g. COL → transportation). */
	subtype?: string;
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
	/** Whether the requesting user may create comparisons of this type (§30). */
	can_create?: boolean;
}

export interface ResolvedItem {
	type: string;
	id: string;
	title: string;
	description?: string;
	image_url?: string;
	url?: string;
	position?: number;
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
	author_name?: string | null;
	author_username?: string | null;
	author_display_name?: string | null;
	author_avatar_url?: string | null;
	space_name?: string | null;
	space_slug?: string | null;
	created_at?: string;
	updated_at?: string;
	publish_at?: string;
	published_at?: string;
	read_time_minutes?: number;
	view_count?: number;
	comment_count?: number;
	share_count?: number;
	reshare_count?: number;
	/** §34 cover/hero media — resolved by the JCE detail endpoint. */
	featured_image_url?: string | null;
	header_image_url?: string | null;
}

export interface JCEComparisonMetadata {
	content_id: string;
	depth: 'quick' | 'detailed';
	promoted_from_id?: string | null;
	promoted_at?: string | null;
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
	/** The entity reference backing this row (when the item is entity-linked)
	 *  — part of the client dedup key alongside position + title. */
	item_ref?: Record<string, unknown> | null;
	dimensions: MatrixDimension[];
}

export interface MatrixDimension {
	dimension_id: string;
	label: string;
	type: string;
	value?: string | null;
	value_rating?: number | null;
	value_boolean?: boolean | null;
	/** ISO code for currency dimensions — from the source field's
	 *  {currency_code} option (§58). Resolve via the active currency list. */
	currency_code?: string;
	/** §112 — true when value_rating was normalized to currency_code
	 *  (the fx display base) from the item's local currency. */
	normalized?: boolean;
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
	config?: Record<string, unknown>;
}

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
	config?: Record<string, unknown>;
	created_at?: string;
	updated_at?: string;
}

export interface FreshnessCell {
	dimension_id: string;
	label: string;
	freshness_score: number;
	source_name?: string;
	source_kind?: string;
	retrieved_at?: string | null;
	is_stale: boolean;
}

export interface FreshnessItem {
	item_id: string;
	item_title: string;
	freshness_score: number;
	cells: FreshnessCell[];
}

export interface FreshnessSummary {
	content_id: string;
	overall_freshness_score: number;
	stale_count: number;
	total_count: number;
	by_item: FreshnessItem[];
}

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
	updated_at?: string;
}
