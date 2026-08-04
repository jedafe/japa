import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { RemittanceCorridor } from '$lib/types';

/**
 * Remittance Rate Tracker at /tools/remittance (Section 74, ADR-075).
 * SSR-fetches the supported corridors so the corridor selector renders
 * without a client round-trip. Rates, history, and alerts are fetched
 * client-side after the user selects a corridor.
 *
 * Ref: japaconnect-engineering/sections/74-remittance-tracker.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let corridors: RemittanceCorridor[] = [];
	let disabled = false;
	try {
		corridors = (await apiList<RemittanceCorridor>(ctx, '/remittance/corridors')).items;
	} catch (e: unknown) {
		const code = (e as { code?: string })?.code;
		const status = (e as { status?: number })?.status;
		if (code === 'remittance_disabled' || status === 404) {
			disabled = true;
		}
	}

	return { corridors, disabled, authed };
};
