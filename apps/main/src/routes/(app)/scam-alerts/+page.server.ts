// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { ScamReport, ScamAlertSubscription, ScamType, ScamPlatform, GeoCountry } from '$lib/types';

const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['scam_alert.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Scam Alerts are not available' });
	}

	const ctx = apiContext(event);
	const dest = event.url.searchParams.get('dest') ?? '';
	const rawScamType = event.url.searchParams.get('scam_type') ?? '';
	const submitted = event.url.searchParams.get('submitted') === '1';

	const params = new URLSearchParams({ page: '1', limit: String(PAGE_SIZE) });
	if (dest) params.set('dest', dest);
	if (rawScamType) params.set('scam_type', rawScamType);

	// Countries, the schema catalog, and the filtered alert list are
	// independent — one wave, each degrading to its empty fallback.
	const [countries, schema, alertsRes, mySubscriptions] = await Promise.all([
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items)
			.catch(() => [] as GeoCountry[]),
		apiGet<{ scam_types: { value: string; label: string }[]; platforms: { value: string; label: string }[] }>(ctx, '/scam-alerts/schema').catch(() => null),
		apiList<ScamReport>(ctx, `/scam-alerts?${params.toString()}`)
			.then((r) => ({ ok: true as const, items: r.items }))
			.catch(() => ({ ok: false as const, items: [] as ScamReport[] })),
		event.locals.user
			? apiGet<ScamAlertSubscription[]>(ctx, '/me/scam-alert-subscriptions').catch(() => null)
			: Promise.resolve(null)
	]);

	const scamTypes = (schema?.scam_types ?? []).map((o) => o.value as ScamType);
	const scamPlatforms = (schema?.platforms ?? []).map((o) => o.value as ScamPlatform);

	// A ?scam_type= outside the canonical enum can never match a row — drop
	// it from the echoed filters so the tab bar doesn't render a phantom
	// active tab (the backend already returns an empty list for it).
	const scam_type = scamTypes.length === 0 || scamTypes.includes(rawScamType as ScamType) ? rawScamType : '';

	if (alertsRes.ok) {
		return { alerts: alertsRes.items, mySubscriptions: mySubscriptions ?? [], pageSize: PAGE_SIZE, loadError: false, filters: { dest, scam_type }, submitted, countries, scamTypes, scamPlatforms };
	}
	return {
		alerts: [] as ScamReport[],
		pageSize: PAGE_SIZE,
		loadError: true,
		filters: { dest, scam_type },
		submitted,
		mySubscriptions: mySubscriptions ?? [],
		countries,
		scamTypes,
		scamPlatforms
	};
};
