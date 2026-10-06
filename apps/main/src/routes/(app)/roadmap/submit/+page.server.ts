import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { RoadmapBoard, RoadmapCategory } from '$lib/types';

/**
 * Submit-a-request page — auth required (roadmap.submit). Categories come
 * from GET /roadmap/board so the select is API-driven (Rule 16).
 * Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['roadmap.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Roadmap is not available' });
	}

	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent('/roadmap/submit')}`);
	}

	// roadmap.submissions_enabled gates the form (§113) — the backend still
	// enforces on POST; this just shows the closed state up front.
	const submissionsEnabled =
		(event.locals.siteSettings?.['roadmap.submissions_enabled'] as boolean | undefined) ?? true;

	const ctx = apiContext(event);
	let categories: RoadmapCategory[] = [];
	try {
		const board = await apiGet<RoadmapBoard>(ctx, '/roadmap/board');
		categories = board?.categories ?? [];
	} catch {
		// Categories degrade to an empty list — the field is optional.
	}

	return { categories, submissionsEnabled };
};
