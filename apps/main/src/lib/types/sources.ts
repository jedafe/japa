/**
 * Source directory + source detail types for the Main App.
 * Mirrors the Go backend's public source API contract.
 */

export interface LinkedEntity {
	type: string;
	slug: string;
	name: string;
	href: string;
}

export interface Source {
	id: string;
	name: string;
	slug: string;
	description?: string;
	logo_url?: string | null;
	keywords?: string[];
	tag_slugs?: Record<string, string>;
	domain: string;
	source_url?: string;
	category: string;
	source_type: string;
	source_kind: string;
	trust_level: number;
	follower_count: number;
	post_count: number;
	is_following?: boolean;
	is_blocked?: boolean;
	linked_entity?: LinkedEntity | null;
}

export interface SourceItem {
	id: string;
	entity_type: string;
	title: string;
	excerpt?: string | null;
	original_url: string;
	href: string;
	published_at: string;
	view_count: number;
	comment_count: number;
	share_count: number;
	vote_score: number;
}

export interface SourceItemEnvelope {
	items: SourceItem[];
	meta: { page_size?: number; page_offset?: number; total?: number };
	sort: string;
	entity_type: string;
}

export interface SourceEntityType {
	value: string;
	label: string;
}

export interface SourceOption {
	value: string;
	label: string;
}

export interface SourceMeta {
	categories: string[];
	source_types: string[];
	entity_types?: SourceEntityType[];
	sorts?: SourceOption[];
	item_sorts?: SourceOption[];
}

export interface SourceCollection {
	slug: string;
	name: string;
	description?: string | null;
	sources: Source[];
}
