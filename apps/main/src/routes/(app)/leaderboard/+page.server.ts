import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { LeaderboardEntry } from '$lib/types';

/**
 * Leaderboard page at /leaderboard (Section 64, ADR-080).
 * SSR-fetches the top users by points.
 *
 * Ref: japaconnect-engineering/sections/64-gamification.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let entries: LeaderboardEntry[] = [];
	let disabled = false;
	try {
		entries = (await apiList<LeaderboardEntry>(ctx, '/gamification/leaderboard?limit=50')).items;
	} catch (e: unknown) {
		const code = (e as { code?: string })?.code;
		const status = (e as { status?: number })?.status;
		if (code === 'gamification_disabled' || status === 404) {
			disabled = true;
		}
	}

	return { entries, disabled, authed };
};
