import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { GeoSpaceOption, ProcessingStat, ProcessingTrendPoint } from '$lib/types';

/**
 * Aggregate SEO page — §83/§93: /processing-times/[country]/[visa_type].
 *
 * Public, SSR-rendered crowd-sourced processing-time intelligence for one
 * destination × visa-category corridor. [country] is a geo space slug
 * (e.g. south-africa); [visa_type] is a hyphenated category slug
 * (e.g. skilled-worker → skilled_worker).
 *
 * Optional filters via query params (shareable, SSR-rendered):
 *   ?origin={space_id|slug}  — origin country
 *   ?consulate={text}        — consulate city
 *   ?months={n}              — stats window (default backend behaviour)
 *
 * API: GET /visa-timelines/stats?dest=&visa=&origin=&consulate=&months=
 *      GET /visa-timelines/stats/trend?dest=&visa=&months=12
 *      GET /visa-timelines/stats?dest= — sibling visa categories for
 *      interlinking (SEO).
 */
export const load: PageServerLoad = async (event) => {
	const tool = getTool('processing-times')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			country: null,
			origins: [],
			visaCategory: event.params.visa_type,
			stats: [],
			trend: [],
			siblingCategories: [] as string[],
			originId: null,
			consulate: null,
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	let countries: GeoSpaceOption[] = [];
	try {
		countries = await loadCountries(ctx);
	} catch {
		countries = [];
	}

	const countrySlug = event.params.country.toLowerCase();
	const country = countries.find((c) => c.slug === countrySlug);
	if (!country) {
		throw error(404, { code: 'not_found', message: 'Unknown destination country' });
	}

	// URL slug → category value: skilled-worker → skilled_worker.
	const visaCategory = event.params.visa_type.toLowerCase().replace(/-/g, '_');

	// Optional filters.
	const originParam = event.url.searchParams.get('origin');
	let originId: string | null = null;
	if (originParam) {
		// Accept either a space id or a slug.
		originId =
			countries.find((c) => c.id === originParam)?.id ??
			countries.find((c) => c.slug === originParam.toLowerCase())?.id ??
			null;
	}
	const consulate = event.url.searchParams.get('consulate') || null;
	const monthsParam = event.url.searchParams.get('months');
	const months = monthsParam && /^\d{1,3}$/.test(monthsParam) ? monthsParam : null;

	// Corridor stats, 12-month trend, and sibling visa categories (SEO
	// interlinking) are independent — one wave.
	const statsParams = new URLSearchParams({ dest: country.id, visa: visaCategory });
	if (originId) statsParams.set('origin', originId);
	if (consulate) statsParams.set('consulate', consulate);
	if (months) statsParams.set('months', months);

	const trendParams = new URLSearchParams({ dest: country.id, visa: visaCategory, months: '12' });
	if (consulate) trendParams.set('consulate', consulate);

	const [stats, trend, allSiblingStats] = await Promise.all([
		apiList<ProcessingStat>(ctx, `/visa-timelines/stats?${statsParams}`)
			.then((r) => r.items)
			.catch(() => [] as ProcessingStat[]),
		apiList<ProcessingTrendPoint>(ctx, `/visa-timelines/stats/trend?${trendParams}`)
			.then((r) => r.items)
			.catch(() => [] as ProcessingTrendPoint[]),
		apiList<ProcessingStat>(ctx, `/visa-timelines/stats?dest=${country.id}`)
			.then((r) => r.items)
			.catch(() => [] as ProcessingStat[])
	]);

	const siblingCategories = [...new Set(allSiblingStats.map((s) => s.visa_category))]
		.filter((v) => v && v !== visaCategory)
		.sort();

	return {
		enabled: true,
		tool,
		country,
		origins: countries,
		visaCategory,
		stats,
		trend,
		siblingCategories,
		originId,
		consulate,
		user: event.locals.user
	};
};
