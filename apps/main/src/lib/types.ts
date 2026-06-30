/**
 * Shared TypeScript types for the main site.
 * Keep payload shapes here so client and server code agree on them.
 */

export interface FeatureFlags {
	[flag: string]: boolean;
}

export interface HealthStatus {
	healthy: boolean;
	flags: FeatureFlags;
}

/** Space (university, neighborhood, etc.) returned by the API. */
export interface Space {
	id: string;
	slug: string;
	name: string;
	space_type: string;
	description?: string;
	country?: string;
	cover_image?: string;
	logo_url?: string;
	world_ranking?: number;
	created_at: string;
}

/** Custom field value attached to a space. */
export interface CustomFieldValue {
	id: string;
	field_id: string;
	entity_type: string;
	entity_id: string;
	value: string;
}

/** Aggregated review metrics for an entity. */
export interface ReviewAggregate {
	entity_type: string;
	entity_id: string;
	average_rating: number;
	review_count: number;
	rating_distribution: Record<string, number>;
}

/** Paginated list response from the API. */
export interface PaginatedResponse<T> {
	data: T[];
	total: number;
	page: number;
	per_page: number;
}