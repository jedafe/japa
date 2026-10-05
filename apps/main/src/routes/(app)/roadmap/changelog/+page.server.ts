import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { RoadmapChangelogItem } from '$lib/types';

/**
 * Public roadmap changelog — GET /api/v1/roadmap/changelog returns every
 * shipped item ordered by shipped_at DESC; the page groups them by month.
 * Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['roadmap.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Roadmap is not available' });
	}

	const ctx = apiContext(event);
	let items: RoadmapChangelogItem[] = [];
	let loadError = false;
	try {
		items = (await apiGet<RoadmapChangelogItem[]>(ctx, '/roadmap/changelog')) ?? [];
	} catch {
		loadError = true;
	}

	return { items, loadError };
};
