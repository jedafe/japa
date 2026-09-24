// Mirrors the publicapi digest DTOs — the extension renders the stable
// public contract only (internal fields never cross the wire).

export interface FeedItem {
	id: string;
	title: string;
	slug: string;
	content_type: string;
	excerpt?: string;
	space_id?: string;
	space_slug?: string;
	space_name?: string;
	author_name?: string;
	author_handle?: string;
	vote_score: number;
	comment_count: number;
	view_count: number;
	tags?: string[];
	published_at?: string;
}

export interface ScamReport {
	id: string;
	scam_type: string;
	destination_space_name?: string;
	destination_space_code?: string;
	platform_used?: string;
	approximate_date?: string;
	amount_range?: string;
	description: string;
	is_pinned: boolean;
	created_at: string;
}

export interface IntelItem {
	id: string;
	kind: 'visa_route' | 'job';
	title: string;
	summary?: string;
	created_at: string;
}

export interface DigestDestination {
	space: { slug: string; name: string; level: string };
	scam_alerts: ScamReport[];
	intel: IntelItem[];
}

export interface Digest {
	for_you: { items: FeedItem[]; source: string };
	destination: DigestDestination | null;
	community: { popular: FeedItem[] };
	generated_at: string;
	cached: boolean;
}
