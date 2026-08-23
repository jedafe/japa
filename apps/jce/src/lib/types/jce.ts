import type { ListMeta } from './index';

export interface ComparisonTypeConfig {
	max_item_count?: number;
	item_ref_types?: string[];
	default_item_ref_type?: string;
	[k: string]: unknown;
}

export interface ComparisonType {
	id: string;
	name: string;
	slug: string;
	description?: string | null;
	icon?: string | null;
	item_label: string;
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
	status: string;
	visibility: string;
	depth: 'quick' | 'detailed';
	author_id?: string;
	space_id?: string;
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
}
