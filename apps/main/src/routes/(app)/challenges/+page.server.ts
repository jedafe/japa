import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Challenge } from '$lib/types';

/**
 * Challenges page at /challenges (Section 64, ADR-080).
 * SSR-fetches active challenges so the page is indexable. For authenticated
 * users, also fetches per-challenge progress (/me/challenges) so the page
 * can render progress bars and completion state (Phase 34.04).
 *
 * Ref: japaconnect-engineering/sections/64-gamification.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Challenges and the user's progress are independent — one wave. The
	// badge catalog goes in wave 2 since it depends on whether any returned
	// challenge actually grants a badge.
	const challengesPromise = apiList<Challenge>(ctx, '/gamification/challenges')
		.then((r) => ({ items: r.items, disabled: false }))
		.catch((e: unknown) => {
			const code = (e as { code?: string })?.code;
			const status = (e as { status?: number })?.status;
			return {
				items: [] as Challenge[],
				disabled: code === 'gamification_disabled' || status === 404
			};
		});
	// Phase 34.04 — fetch the authenticated user's challenge progress so we
	// can render progress bars and reward details. Best-effort: guests and
	// any backend error degrade to an empty map (challenges still render
	// without progress).
	const myProgressPromise = authed
		? apiList<{ id: string; target_count: number; current_count: number; completed: boolean }>(ctx, '/me/challenges')
				.then((r) => r.items)
				.catch(() => [] as { id: string; target_count: number; current_count: number; completed: boolean }[])
		: Promise.resolve([] as { id: string; target_count: number; current_count: number; completed: boolean }[]);

	const [challengesRes, myProgress] = await Promise.all([challengesPromise, myProgressPromise]);
	const challenges = challengesRes.items;
	const disabled = challengesRes.disabled;

	// Badge catalog — used to resolve reward_badge_id to a badge name so the
	// card can show "badge reward" instead of a bare id. Best-effort: when
	// badges are disabled or the call fails the chip falls back to a generic
	// label.
	let badges: Array<{ id: string; name: string }> = [];
	if (!disabled && challenges.some((c) => c.reward_badge_id)) {
		try {
			badges = (await apiList<{ id: string; name: string }>(ctx, '/gamification/badges')).items;
		} catch {
			badges = [];
		}
	}

	return { challenges, disabled, authed, myProgress, badges };
};
