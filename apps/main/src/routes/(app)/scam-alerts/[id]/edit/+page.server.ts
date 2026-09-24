// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { ScamReport, GeoCountry, ScamType, ScamPlatform } from '$lib/types';

/**
 * Edit-scam form at /scam-alerts/[id]/edit — auth-gated, author-or-edit_any.
 * Loads the report by id and the country/schema lists for the form.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const { features } = await event.parent();
	if (!features['scam_alert.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Scam alert board is not available' });
	}

	const ctx = apiContext(event);
	const id = event.params.id;

	let alert: ScamReport;
	try {
		// ?raw=1 keeps authored {{...}} literals unresolved for the edit form.
		alert = await apiGet<ScamReport>(ctx, `/scam-alerts/${id}?raw=1`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Scam alert not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load scam alert' });
	}

	const canEdit =
		alert.reporter_id === event.locals.user.id ||
		event.locals.user.permissions?.includes('scam_alert.edit_any');
	if (!canEdit) {
		throw error(403, { code: 'not_owner', message: 'You can only edit your own scam reports' });
	}

	const [countriesRes, schemaRes] = await Promise.allSettled([
		apiList<GeoCountry>(ctx, '/geo/countries'),
		apiGet<{ scam_types: { value: string; label: string }[]; platforms: { value: string; label: string }[] }>(
			ctx,
			'/scam-alerts/schema'
		)
	]);

	const countries = countriesRes.status === 'fulfilled' ? countriesRes.value.items : [];
	let scamTypes: ScamType[] = [];
	let scamPlatforms: ScamPlatform[] = [];
	if (schemaRes.status === 'fulfilled' && schemaRes.value) {
		scamTypes = (schemaRes.value.scam_types ?? []).map((o) => o.value as ScamType);
		scamPlatforms = (schemaRes.value.platforms ?? []).map((o) => o.value as ScamPlatform);
	}

	return { alert, countries, scamTypes, scamPlatforms, user: event.locals.user, features };
};
