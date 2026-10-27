import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { RoadmapReleaseList } from '$lib/types';

/**
 * Public releases index — GET /api/v1/roadmap/releases returns published
 * releases with computed support_state + the unreleased commit count.
 * Ref: japaconnect-engineering/sections/113-public-product-roadmap.md (ADR-135)
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['roadmap.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Roadmap is not available' });
	}

	const ctx = apiContext(event);
	let releases: RoadmapReleaseList = { releases: [], unreleased_count: 0 };
	let loadError = false;
	try {
		releases = (await apiGet<RoadmapReleaseList>(ctx, '/roadmap/releases')) ?? releases;
	} catch {
		loadError = true;
	}

	return { ...releases, loadError };
};
