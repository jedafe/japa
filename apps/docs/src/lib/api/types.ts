export interface User {
	id: string;
	username: string;
	display_name: string;
	email: string;
	avatar_url: string | null;
	user_group_id: string;
	theme_preference: 'light' | 'dark' | 'system';
	timezone: string;
	onboarding_completed: boolean;
	is_verified: boolean;
	created_at: string;
}

export interface FeatureFlags {
	[key: string]: boolean;
}

export interface DocNavItem {
	id: string;
	title: string;
	slug: string;
	audience: string;
	parent_id: string | null;
	position: number;
	children?: DocNavItem[];
}

export interface DocNavTree {
	audience: string;
	items: DocNavItem[];
}

export interface DocPage {
	id: string;
	title: string;
	slug: string;
	audience: string;
	content: Record<string, unknown>;
	ai_generated: boolean;
	review_status: string;
	parent_id: string | null;
	position: number;
	lang_code: string;
	prev: { title: string; slug: string } | null;
	next: { title: string; slug: string } | null;
	created_at: string;
	updated_at: string;
}

export interface DocPageSummary {
	id: string;
	title: string;
	slug: string;
	audience: string;
	review_status: string;
	parent_id: string | null;
	position: number;
	created_at: string;
}

export interface ApiSpec {
	openapi: string;
	info: { title: string; version: string; description: string };
	paths: Record<string, unknown>;
	components: Record<string, unknown>;
}

export interface PlaygroundRequest {
	method: string;
	url: string;
	headers: Record<string, string>;
	body: string;
}

export interface PlaygroundResponse {
	status: number;
	statusText: string;
	headers: Record<string, string>;
	body: string;
	time: number;
	size: number;
}

export interface PaginatedResponse<T> {
	data: T[];
	total: number;
	page: number;
	per_page: number;
	total_pages: number;
}

export interface SearchResult {
	id: string;
	title: string;
	slug: string;
	audience: string;
	excerpt: string;
	highlights: string[];
	url: string;
}
