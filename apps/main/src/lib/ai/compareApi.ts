import { api } from '$lib/api/client';
import type { ClientResult } from '$lib/api/client';

/**
 * Client wrapper for POST /api/v1/ai/compare-entities (§103 task 7.07 —
 * the direct comparison endpoint that orchestrates the ComparisonEngine).
 * Auth-gated by agent.use; discovery-gated by feature.jce.ai_quick (§30).
 */

export interface CompareEntitiesInput {
	/** Slug of the comparison type (e.g. "universities"). */
	entity_type: string;
	/** Opaque identifiers — selected item titles/labels. */
	entity_ids: string[];
	/** Optional dimension labels the user cares about. */
	comparison_fields?: string[];
	/** Optional focus text guiding the pre-search. */
	focus?: string;
	/** Optional geo space to scope retrieval. */
	geo_space_id?: string;
	/** Tier 3 only — create a draft comparison content row. */
	create_post?: boolean;
}

export interface CompareEntitiesResult {
	tier: 'structured' | 'semi_structured' | 'unstructured' | string;
	entity_type: string;
	entity_ids: string[];
	comparison_fields?: string[];
	existing_dimensions?: string[];
	missing_dimensions?: string[];
	type_id?: string;
	type_name?: string;
	type_slug?: string;
	presearch_level?: string;
	presearch_summary?: string;
	content_id?: string;
	content_status?: string;
	dimension_ids?: string[];
	item_ids?: string[];
	comparison?: {
		comparison?: {
			id: string;
			title: string;
			slug: string;
			status: string;
		};
		matrix?: unknown[];
	};
}

export function compareEntities(
	input: CompareEntitiesInput
): Promise<ClientResult<CompareEntitiesResult>> {
	return api.post<CompareEntitiesResult>('/ai/compare-entities', input);
}
