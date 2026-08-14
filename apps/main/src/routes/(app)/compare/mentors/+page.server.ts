// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { Mentor } from '$lib/types';

/**
 * Mentor comparison at /compare/mentors?ids=a,b,c,d — SSR-fetches
 * GET /compare/mentors?ids=... (max 4). Reuses the §30 Generic Comparison
 * System's side-by-side layout pattern.
 *
 * Feature-flagged via `mentorship.enabled` and `mentorship.comparison` (§102):
 * returns 404 when either flag is disabled.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['mentorship.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Mentorship is not available' });
	}
	if (!features['mentorship.comparison']) {
		throw error(404, { code: 'feature_disabled', message: 'Mentor comparison is not available' });
	}

	const ids = event.url.searchParams.get('ids') ?? '';
	if (!ids) {
		return { mentors: [] as Mentor[], loadError: false };
	}

	const idList = ids.split(',').filter(Boolean);
	if (idList.length < 2 || idList.length > 4) {
		throw error(400, { code: 'invalid_ids', message: 'Provide 2-4 mentor IDs to compare' });
	}

	const ctx = apiContext(event);
	try {
		const res = await apiGet<{ data: Mentor[] }>(ctx, `/compare/mentors?ids=${ids}`);
		return { mentors: res.data ?? [], loadError: false };
	} catch {
		return { mentors: [] as Mentor[], loadError: true };
	}
};
