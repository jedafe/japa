// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Entity Directory amendment)
import { apiContext, apiGet } from '$lib/server/api';
import type { PageServerLoad } from './$types';

export interface EntityKindType {
	slug: string;
	label: string;
	entity_count: number;
}

export interface EntityKind {
	kind: string;
	label: string;
	entity_count: number;
	types?: EntityKindType[];
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	let kinds: EntityKind[] = [];
	try {
		kinds = (await apiGet<EntityKind[]>(ctx, '/wiki/entity-kinds')) ?? [];
	} catch {
		kinds = [];
	}
	return { kinds };
};
