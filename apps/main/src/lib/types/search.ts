/** Search result item from GET /api/v1/search (Section 13). */
export interface SearchResult {
	type: string;
	id: string;
	title: string;
	excerpt: string;
	url: string;
}
