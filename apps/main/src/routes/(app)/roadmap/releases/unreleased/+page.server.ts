import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { RoadmapReleaseEntry } from '$lib/types';

/**
 * Unreleased commit entries — the "In development" queue of public-type
 * changelog entries not yet absorbed into a release.
 * Ref: japaconnect-engineering/sections/113-public-product-roadmap.md (ADR-135)
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['roadmap.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Roadmap is not available' });
	}

	const ctx = apiContext(event);
	let entries: RoadmapReleaseEntry[] = [];
	let loadError = false;
	try {
		entries = (await apiGet<RoadmapReleaseEntry[]>(ctx, '/roadmap/releases/unreleased')) ?? [];
	} catch {
		loadError = true;
	}

	return { entries, loadError };
};
