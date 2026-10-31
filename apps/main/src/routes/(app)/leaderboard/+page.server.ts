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

interface SpaceOption {
	id: string;
	name: string;
	slug?: string;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Phase 34.05 — scope and period selectors. Defaults: global + all_time.
	// Scope is 'global' or 'space:{uuid}' — anything else falls back to global
	// so a malformed query param can never produce a 400 page.
	const rawScope = event.url.searchParams.get('scope') ?? 'global';
	const scope =
		rawScope === 'global' || (rawScope.startsWith('space:') && UUID_RE.test(rawScope.slice(6)))
			? rawScope
			: 'global';
	const rawPeriod = event.url.searchParams.get('period') ?? 'all_time';
	// Whitelist like scope — a malformed period must fall back, not 400.
	const period = ['all_time', 'monthly', 'weekly'].includes(rawPeriod) ? rawPeriod : 'all_time';

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

	// Per-space scope options — the spaces the signed-in member belongs to.
	// member_only=1 keeps super-admin results accurate (they implicitly belong
	// to every space). Guests get the global scope only.
	let spaces: SpaceOption[] = [];
	if (authed && !disabled) {
		try {
			spaces = (
				await apiList<SpaceOption>(ctx, '/me/spaces?member_only=1&page_size=50')
			).items;
		} catch {
			spaces = [];
		}
	}

	return { entries, disabled, authed, scope, period, spaces };
};
