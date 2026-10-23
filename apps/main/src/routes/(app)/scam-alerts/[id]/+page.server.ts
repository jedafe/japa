// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { ScamReport } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();

	// §81: Scam Alerts is a feature-flagged surface.
	if (!features['scam_alert.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Scam Alerts are not available' });
	}

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
		return { alert: null, loadError: true, authed, canEdit: false };
	}

	// §81: the reporter (is_reporter) or a scam_alert.edit_any holder may edit
	// — the detail page needs this to render the Edit affordance that
	// discovers /scam-alerts/[id]/edit.
	const canEdit =
		authed &&
		(alert.is_reporter === true ||
			(event.locals.user?.permissions ?? []).includes('scam_alert.edit_any'));

	return { alert, loadError: false, authed, canEdit };
};
