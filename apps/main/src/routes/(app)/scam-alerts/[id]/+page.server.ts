// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { ScamReport } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const id = event.params.id;
	const authed = !!event.locals.user;

	let alert: ScamReport;
	try {
		alert = await apiGet<ScamReport>(ctx, `/scam-alerts/${id}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Scam alert not found' });
		}
		return { alert: null, loadError: true, authed };
	}

	return { alert, loadError: false, authed };
};
