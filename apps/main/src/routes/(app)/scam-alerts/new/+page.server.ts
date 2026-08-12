// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Space } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	// §81: anonymous reports are allowed when scam_alert.anonymous_reports is
	// enabled (default true). Do NOT force a login redirect here — the backend
	// handles auth-or-anonymous. The submit form works for guests.
	const ctx = apiContext(event);

	let countries: Space[] = [];
	try {
		const { items } = await apiList<Space>(ctx, '/spaces?level=country');
		countries = items;
	} catch {
		countries = [];
	}

	return { countries };
};
