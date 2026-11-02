import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { Badge, BadgeCollection } from '$lib/types';

/**
 * Achievement centre at /badges (gamification v2 catalogue).
 * SSR-fetches the member's badge collection — the full active catalogue
 * grouped by category with earned state + progress. Guests get the public
 * catalogue without earned/progress state.
 */

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let collection: BadgeCollection | null = null;
	let allBadges: Badge[] = [];
	let disabled = false;

	if (authed) {
		try {
			collection = await apiGet<BadgeCollection>(ctx, '/me/badge-collection');
		} catch (e: unknown) {
			const code = (e as { code?: string })?.code;
			const status = (e as { status?: number })?.status;
			if (code === 'gamification_disabled' || status === 404) {
				disabled = true;
			}
		}
	} else {
		// Guests get the public catalogue rendered as locked tiles —
		// the route is public (gamification.enabled gate only).
		try {
			allBadges = (await apiList<Badge>(ctx, '/gamification/badges')).items;
		} catch (e: unknown) {
			const code = (e as { code?: string })?.code;
			if (code === 'gamification_disabled') disabled = true;
		}
	}

	return {
		authed,
		disabled,
		collection,
		allBadges,
	};
};
