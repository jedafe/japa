// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { GeoCountry, ScamType, ScamPlatform } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const { features, user } = await event.parent();

	// §81: Scam Alerts is a feature-flagged surface.
	if (!features['scam_alert.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Scam Alerts are not available' });
	}

	// §81: anonymous reports are allowed when scam_alert.anonymous_reports is
	// enabled (default true). Do NOT force a login redirect here — the backend
	// handles auth-or-anonymous. The submit form works for guests.
	const ctx = apiContext(event);

	const [countries, schema] = await Promise.all([
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items)
			.catch(() => [] as GeoCountry[]),
		apiGet<{ scam_types: { value: string; label: string }[]; platforms: { value: string; label: string }[] }>(
			ctx,
			'/scam-alerts/schema'
		).catch(() => null)
	]);
	const scamTypes = (schema?.scam_types ?? []).map((o) => o.value as ScamType);
	const scamPlatforms = (schema?.platforms ?? []).map((o) => o.value as ScamPlatform);

	return { countries, scamTypes, scamPlatforms, user, features };
};
