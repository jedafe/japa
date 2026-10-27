import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { RoadmapReleaseDetail } from '$lib/types';

/**
 * Release detail — GET /api/v1/roadmap/releases/{slug} returns the release
 * notes body, shipped roadmap items, and commit entries ordered by type.
 * ?scope= filters items + entries by extension scope.
 * Ref: japaconnect-engineering/sections/113-public-product-roadmap.md (ADR-135)
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['roadmap.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Roadmap is not available' });
	}

	const scope = event.url.searchParams.get('scope') ?? '';
	const qs = scope ? `?scope=${encodeURIComponent(scope)}` : '';

	const ctx = apiContext(event);
	let release: RoadmapReleaseDetail | null = null;
	let loadError = false;
	try {
		release = await apiGet<RoadmapReleaseDetail>(ctx, `/roadmap/releases/${event.params.slug}${qs}`);
	} catch {
		loadError = true;
	}
	if (!release && !loadError) {
		throw error(404, { code: 'not_found', message: 'Release not found' });
	}

	return { release, scope, loadError };
};
