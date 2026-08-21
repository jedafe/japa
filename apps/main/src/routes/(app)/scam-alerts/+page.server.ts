// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { ScamReport, ScamType, ScamPlatform, GeoCountry } from '$lib/types';

const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['scam_alert.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Scam Alerts are not available' });
	}

	const ctx = apiContext(event);
	const dest = event.url.searchParams.get('dest') ?? '';
	const scam_type = event.url.searchParams.get('scam_type') ?? '';

	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	let scamTypes: ScamType[] = [];
	let scamPlatforms: ScamPlatform[] = [];
	try {
		const schema = await apiGet<{ scam_types: { value: string; label: string }[]; platforms: { value: string; label: string }[] }>(ctx, '/scam-alerts/schema');
		scamTypes = (schema.scam_types ?? []).map((o) => o.value as ScamType);
		scamPlatforms = (schema.platforms ?? []).map((o) => o.value as ScamPlatform);
	} catch {
		scamTypes = [];
		scamPlatforms = [];
	}

	const params = new URLSearchParams({ page: '1', limit: String(PAGE_SIZE) });
	if (dest) params.set('dest', dest);
	if (scam_type) params.set('scam_type', scam_type);

	try {
		const { items } = await apiList<ScamReport>(ctx, `/scam-alerts?${params.toString()}`);
		return { alerts: items, pageSize: PAGE_SIZE, loadError: false, filters: { dest, scam_type }, countries, scamTypes, scamPlatforms };
	} catch {
		return {
			alerts: [] as ScamReport[],
			pageSize: PAGE_SIZE,
			loadError: true,
			filters: { dest, scam_type },
			countries,
			scamTypes,
			scamPlatforms
		};
	}
};
