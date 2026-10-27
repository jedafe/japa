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
	/** Multi-category set (intel-channels Phase 1) — present on proposal and
	 *  admin payloads; `category` remains the primary/first entry. */
	categories?: string[];
	source_type: string;
	source_kind: string;
	trust_level: number;
	follower_count: number;
	post_count: number;
	is_following?: boolean;
	is_blocked?: boolean;
	linked_entity?: LinkedEntity | null;
	/** §106 video channels only — the channel account username, used to link
	 *  the claim/report form. The channel has no public /u/ profile. */
	channel_username?: string;
	/** Feed entity types this source currently publishes (detail endpoint
	 *  only) — scopes the entity-type filter pills to types with items. */
	entity_types?: string[];
	/** Detail-endpoint provenance fields for the About section (v2 spec). */
	source_license?: string | null;
	attribution_required?: boolean;
	crawl_frequency?: string;
	last_crawled_at?: string | null;
}

export interface SourceItem {
	id: string;
	content_id?: string;
	entity_type: string;
	title: string;
	excerpt?: string | null;
	original_url: string;
	href: string;
	thumbnail_url?: string | null;
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
	/** Curated proposal-catalog categories. */
	categories: string[];
	/** Propose-form subset of categories — domains whose channels reach
	 *  content without AI extraction (job-family, news/article, video).
	 *  buildProposeParams rejects proposals outside it. */
	submittable_categories?: string[];
	/** Full extraction-domain taxonomy (schemas.RegisteredCategories) —
	 *  the propose form's category picker uses this superset. */
	admin_categories?: string[];
	source_types: string[];
	/** Publisher-kind options ({value,label}) for the propose form's
	 *  source_kind select and the directory ?source_kind= filter — distinct
	 *  from source_types (crawl mechanism, no longer user-facing). */
	source_kinds?: SourceOption[];
	/** Propose-form subset of source_kinds — entity-stub kinds
	 *  (institution, individual) are excluded; the propose endpoint rejects
	 *  them server-side too. */
	submittable_source_kinds?: SourceOption[];
	/** intel_source_channels.pipeline CHECK set (extract|curate|hybrid). */
	channel_pipelines?: string[];
	entity_types?: SourceEntityType[];
	sorts?: SourceOption[];
	item_sorts?: SourceOption[];
}

// --- Probe preview (POST /api/v1/sources/probe) ---------------------------
// Mirrors probe.Report in the Go backend (intel-channels design §2).

export interface ProbeDetectedMethod {
	method: string;
	confidence: number;
	evidence: string;
}

export interface ProbeChannelProposal {
	label: string;
	entry_url: string;
	crawl_method: string;
	entity_types?: string[];
	/** extract | curate | hybrid */
	pipeline: string;
	confidence: number;
	provider_config?: Record<string, unknown>;
	relevance_topics?: string[];
}

export interface ProbeRobotVerdict {
	fetched: boolean;
	allowed: boolean;
	crawl_delay_secs?: number;
	disallowed_paths?: string[];
	sitemap_urls?: string[];
}

export interface ProbeReport {
	final_url: string;
	detected_kinds?: string[];
	methods?: ProbeDetectedMethod[] | null;
	channels?: ProbeChannelProposal[] | null;
	jsonld_types?: string[];
	feed_urls?: string[];
	sitemap_urls?: string[];
	has_websub: boolean;
	requires_js: boolean;
	robots: ProbeRobotVerdict;
	warnings?: string[];
	probed_at: string;
}

export interface SourceCollection {
	slug: string;
	name: string;
	description?: string | null;
	sources: Source[];
}
