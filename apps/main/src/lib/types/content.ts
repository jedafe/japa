/**
 * Content article shape — matches the backend GetContentBySlugRow.
 * Used by /blog/[slug] and /content/[slug] routes.
 */
export interface ContentArticle {
	id: string;
	title: string;
	slug: string;
	content_type: string;
	body: string;
	body_html: string;
	excerpt?: string;
	status: string;
	visibility: string;
	read_time_minutes: number;
	view_count: number;
	comment_count: number;
	share_count: number;
	reshare_count: number;
	published_at: string;
	created_at: string;
	updated_at: string;
	author_id?: string;
	author_name?: string;
	author_avatar_url?: string | null;
	space_id?: string | null;
	space_name?: string | null;
	space_slug?: string | null;
	header_image_url?: string | null;
	featured_image_url?: string | null;
	tags?: string[];
	// Series fields — optional, present when the article is part of a series.
	series_id?: string | null;
	series_title?: string | null;
	series_part?: number | null;
	series_total?: number | null;
	series_prev_slug?: string | null;
	series_next_slug?: string | null;
}
