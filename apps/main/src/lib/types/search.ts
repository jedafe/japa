/** Search result item from GET /api/v1/search (Section 13 + ADR-017).
 *
 * Per §54 the response shape is { type, item } where item holds the
 * type-specific fields. When no type filter is active, the API returns
 * grouped results (ADR-017: top 2-3 per category) in the `groups` field.
 * When a specific type is requested, `results` is populated.
 */
export interface SearchResultItem {
	id: string;
	title?: string;
	excerpt?: string;
	url?: string;
	content_type?: string;
	slug?: string;
	username?: string;
	display_name?: string;
	avatar_url?: string;
	name?: string;
	level?: string;
	member_count?: number;
	content_count?: number;
}

export interface SearchResult {
	type: string;
	item: SearchResultItem;
}

export interface ResultGroup {
	type: string;
	results: SearchResult[];
	total: number;
}

export interface SearchResponse {
	results?: SearchResult[];
	groups?: ResultGroup[];
	total: number;
	took_ms: number;
}

/** Autocomplete suggestion from GET /api/v1/search/autocomplete (§54). */
export interface AutocompleteSuggestion {
	type: string;
	id: string;
	title?: string;
	excerpt?: string;
	url?: string;
	content_type?: string;
	level?: string;
	slug?: string;
	username?: string;
	display_name?: string;
	avatar_url?: string;
	name?: string;
	member_count?: number;
	content_count?: number;
}
