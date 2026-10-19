// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { VisaProcessingStats, TrendPoint, GeoCountry } from '$lib/types';

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

	const qs = new URLSearchParams();
	if (filters.dest) qs.set('dest', filters.dest);
	if (filters.origin) qs.set('origin', filters.origin);
	if (filters.visa) qs.set('visa', filters.visa);
	if (filters.consulate) qs.set('consulate', filters.consulate);
	if (filters.months) qs.set('months', filters.months);
	const query = qs.toString();

	let loadError = false;
	const [countries, stats, trend] = await Promise.all([
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items)
			.catch(() => [] as GeoCountry[]),
		apiGet<VisaProcessingStats[]>(
			ctx,
			'/visa-timelines/stats' + (query ? `?${query}` : '')
		).catch(() => {
			loadError = true;
			return [] as VisaProcessingStats[];
		}),
		apiGet<TrendPoint[]>(
			ctx,
			'/visa-timelines/stats/trend' + (query ? `?${query}` : '')
		).catch(() => [] as TrendPoint[])
	]);

	return { stats, trend, countries, loadError, filters, authed };
};
