import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { LeaderboardEntry } from '$lib/types';

/**
 * Leaderboard page at /leaderboard (Section 64, ADR-080).
 * SSR-fetches the top users by points. Supports scope (global/per-space) and
 * period (weekly/monthly/all-time) selectors via query params (Phase 34.05).
 *
 * Ref: japaconnect-engineering/sections/64-gamification.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Phase 34.05 — scope and period selectors. Defaults: global + all_time.
	const scope = event.url.searchParams.get('scope') ?? 'global';
	const period = event.url.searchParams.get('period') ?? 'all_time';

	let entries: LeaderboardEntry[] = [];
	let disabled = false;
	try {
		const qs = new URLSearchParams({ limit: '50', scope, period });
		entries = (await apiList<LeaderboardEntry>(ctx, `/gamification/leaderboard?${qs.toString()}`)).items;
	} catch (e: unknown) {
		const code = (e as { code?: string })?.code;
		const status = (e as { status?: number })?.status;
		if (code === 'gamification_disabled' || status === 404) {
			disabled = true;
		}
	}

	return { entries, disabled, authed, scope, period };
};
