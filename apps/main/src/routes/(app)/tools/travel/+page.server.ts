// Ref: japaconnect-engineering/sections/67-travel-search.md (ADR-085)
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { CatalogOption } from '$lib/types';

/**
 * Travel Search at /tools/travel (Section 67, ADR-085).
 * The backend gate (travel.enabled) is enforced inside the handler; the
 * page renders the search forms SSR and fetches results client-side.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['travel.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Travel Search is not available' });
	}

	const ctx = apiContext(event);

	// Cabin-class options (Rule 16 — option lists come from the backend
	// catalog endpoint, never hardcoded frontend arrays). Degrades
	// non-fatally.
	const cabinClasses = await apiGet<CatalogOption[]>(ctx, '/catalogs/travel.cabin_class')
		.then((v) => v ?? [])
		.catch(() => [] as CatalogOption[]);

	return { cabinClasses };
};
