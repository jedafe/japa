/**
 * Content article shape — matches the backend GetContentBySlugRow.
 * Used by /blog/[slug] and /content/[slug] routes.
 */
export interface ContentAttribution {
	id: string;
	name: string;
	slug: string;
	logo_url?: string | null;
	source_url: string;
	trust_level: number;
}

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
	author_username?: string;
	author_avatar_url?: string | null;
	space_id?: string | null;
	space_name?: string | null;
	space_slug?: string | null;
	/** Category slug when content_type === 'guide' (§14 guide amendment). */
	guide_category?: string | null;
	/** Admin-configured category label resolved server-side (e.g. 'Visa Guide'). */
	guide_category_label?: string | null;
	/** Category-specific custom field values (decoded JSON object). */
	field_values?: Record<string, unknown> | null;
	/** Journey-aware recommendation metadata (§32). */
	journey_stage?: string | null;
	visa_route?: string | null;
	source_country?: string | null;
	target_country?: string | null;
	reaction_count?: number;
	bookmark_count?: number;
	header_image_url?: string | null;
	featured_image_url?: string | null;
	/** §106 video — linked video_sources row ID (content_type === 'video').
	 *  The single page resolves it via GET /videos/{id} to render the embed. */
	video_source_id?: string | null;
	tags?: string[];
	/** Source attribution for content promoted from the intelligence spine. */
	source?: ContentAttribution;
	// Series fields — optional, present when the article is part of a series.
	series_id?: string | null;
	series_title?: string | null;
	series_slug?: string | null;
	series_part?: number | null;
	series_total?: number | null;
	series_prev_slug?: string | null;
	series_next_slug?: string | null;
}

/** A single part in a blog series (returned by /blog/series/{id}/posts). */
export interface SeriesPost {
	content_id: string;
	position: number;
	title: string;
	slug: string;
	status: string;
	created_at: string;
}

/**
 * Row shape of GET /content?type=guide (ListContentsByTypeRow) — used by the
 * /guides hub. Ref: §14 Public Read Path addendum.
 */
export interface GuideListItem {
	id: string;
	title: string;
	slug: string;
	content_type: string;
	author_id?: string | null;
	author_username?: string | null;
	author_display_name?: string | null;
	author_avatar_url?: string | null;
	space_id?: string | null;
	space_name?: string | null;
	guide_category?: string | null;
	journey_stage?: string | null;
	visa_route?: string | null;
	source_country?: string | null;
	target_country?: string | null;
	purpose?: string | null;
	read_time_minutes?: number;
	view_count?: number;
	comment_count?: number;
	share_count?: number;
	reshare_count?: number;
	reaction_count?: number;
	bookmark_count?: number;
	vote_score?: number;
	published_at?: string | null;
	created_at: string;
	header_image_url?: string | null;
	featured_image_url?: string | null;
	/** Raw contents.body — TipTap JSON text for forum threads; used to derive
	 *  the card cover image when no explicit featured/header media exists. */
	body?: string;
	tags?: string[];
}

/** Lightweight organization badge used by the social post wrapper. */
export interface PostOrganization {
	id: string;
	name: string;
	slug: string;
	logo_url?: string | null;
}

/**
 * GET /post/{content_type}/{slug} — social wrapper detail row.
 * Mirrors ContentArticle but adds the normalized identity/reaction fields
 * needed by the daily feed card and the social post page. Ref: §23 daily card
 * amendment and docs/superpowers/specs/2026-09-10-daily-feed-style-and-social-post-page-design.md
 */
export interface PostDetail extends ContentArticle {
	status: string;
	visibility: string;
	is_pinned?: boolean;
	is_locked?: boolean;
	canonical_url?: string | null;
	meta_title?: string | null;
	meta_description?: string | null;
	og_image_url?: string | null;
	seo_indexable?: boolean;
	reaction_count?: number;
	bookmark_count?: number;
	space_avatar_url?: string | null;
	organization?: PostOrganization | null;
	duration_secs?: number | null;
}

/**
 * GET /content/{id}/analytics — owner/admin content stats (Section 40).
 * Consumed by the content single page for the author only.
 */
export interface ContentAnalytics {
	daily_stats?: {
		day: string;
		views?: number;
		reactions?: number;
		comments?: number;
		shares?: number;
		unique_viewers?: number;
	}[];
	views?: number;
	reactions?: number;
	comments?: number;
	shares?: number;
	unique_viewers?: number;
}
