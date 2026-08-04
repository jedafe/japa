import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import { ApiError } from '$lib/server/api';
import type { ChecklistSelector } from '$lib/types';

/**
 * Visa Checklist tool page at /tools/visa-checklist (Section 70, ADR-070).
 * SSR-fetches the selector options (origins + destinations with nested visa
 * types) so the 3-step selector renders without a client round-trip.
 *
 * Ref: japaconnect-engineering/sections/70-visa-checklist.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let selector: ChecklistSelector | null = null;
	let disabled = false;
	try {
		selector = await apiGet<ChecklistSelector>(ctx, '/checklist/selector');
	} catch (e) {
		if (e instanceof ApiError && (e.code === 'checklist_disabled' || e.status === 404)) {
			disabled = true;
		}
		// Other errors: leave selector null; page shows a loading/error state.
	}

	return { selector, disabled, authed };
};
