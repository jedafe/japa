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
	domain: string;
	category: string;
	source_type: string;
	source_kind: string;
	trust_level: number;
	follower_count: number;
	post_count: number;
	is_following?: boolean;
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
