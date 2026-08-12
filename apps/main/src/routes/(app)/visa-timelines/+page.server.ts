// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { VisaProcessingStats, TrendPoint, Space } from '$lib/types';

/**
 * Visa Timeline aggregate stats at /visa-timelines (Section 83).
 * SSR-fetches GET /visa-timelines/stats and GET /visa-timelines/stats/trend,
 * passing through the dest/origin/visa/consulate/months query params so the
 * page renders with the active filters applied without a client round-trip.
 *
 * The stats endpoint returns an array of period rows (one per month bucket);
 * the page derives the latest period for headline display.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const filters = {
		dest: event.url.searchParams.get('dest') ?? '',
		origin: event.url.searchParams.get('origin') ?? '',
		visa: event.url.searchParams.get('visa') ?? '',
		consulate: event.url.searchParams.get('consulate') ?? '',
		months: event.url.searchParams.get('months') ?? ''
	};

	let countries: Space[] = [];
	try {
		const { items } = await apiList<Space>(ctx, '/spaces?level=country');
		countries = items;
	} catch {
		countries = [];
	}

	const qs = new URLSearchParams();
	if (filters.dest) qs.set('dest', filters.dest);
	if (filters.origin) qs.set('origin', filters.origin);
	if (filters.visa) qs.set('visa', filters.visa);
	if (filters.consulate) qs.set('consulate', filters.consulate);
	if (filters.months) qs.set('months', filters.months);
	const query = qs.toString();

	let stats: VisaProcessingStats[] = [];
	let trend: TrendPoint[] = [];
	let loadError = false;
	try {
		stats = await apiGet<VisaProcessingStats[]>(ctx, `/visa-timelines/stats${query ? `?${query}` : ''}`);
	} catch {
		loadError = true;
	}
	try {
		const trendQuery = qs.toString();
		trend = await apiGet<TrendPoint[]>(ctx, `/visa-timelines/stats/trend${trendQuery ? `?${trendQuery}` : ''}`);
	} catch {
		trend = [];
	}

	return { stats, trend, countries, loadError, filters, authed };
};
