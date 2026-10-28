import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { RoadmapBoard } from '$lib/types';

/**
 * Public product roadmap board — GET /api/v1/roadmap/board (OptionalAuth;
 * the backend returns per-item has_voted for authenticated viewers via the
 * forwarded bearer token in ApiContext).
 * Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
 */
export const load: PageServerLoad = async (event) => {
	// roadmap.enabled gates the whole /roadmap/* route group server-side
	// (features.Gate → 404); mirror that for the page itself.
	const { features } = await event.parent();
	if (!features?.['roadmap.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Roadmap is not available' });
	}

	const ctx = apiContext(event);
	const category = event.url.searchParams.get('category') ?? '';
	const sort = event.url.searchParams.get('sort') === 'recent' ? 'recent' : 'votes';

	// Custom invalidation key so client-side mutations (votes, moderator
	// status changes) re-run this load without depending on the fetch URL.
	event.depends('roadmap:board');

	let board: RoadmapBoard | null = null;
	let loadError = false;
	try {
		board = await apiGet<RoadmapBoard>(
			ctx,
			`/roadmap/board${category ? `?category=${encodeURIComponent(category)}` : ''}`
		);
	} catch {
		loadError = true;
	}

	return { board, loadError, category, sort };
};
