import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Challenge } from '$lib/types';

/**
 * Challenges page at /challenges (Section 64, ADR-080).
 * SSR-fetches active challenges so the page is indexable.
 *
 * Ref: japaconnect-engineering/sections/64-gamification.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let challenges: Challenge[] = [];
	let disabled = false;
	try {
		challenges = (await apiList<Challenge>(ctx, '/gamification/challenges')).items;
	} catch (e: unknown) {
		const code = (e as { code?: string })?.code;
		const status = (e as { status?: number })?.status;
		if (code === 'gamification_disabled' || status === 404) {
			disabled = true;
		}
	}

	return { challenges, disabled, authed };
};
