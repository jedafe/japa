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

	// Corridor stats.
	let stats: ProcessingStat[] = [];
	{
		const params = new URLSearchParams({ dest: country.id, visa: visaCategory });
		if (originId) params.set('origin', originId);
		if (consulate) params.set('consulate', consulate);
		if (months) params.set('months', months);
		try {
			stats = (await apiList<ProcessingStat>(ctx, `/visa-timelines/stats?${params}`)).items;
		} catch {
			stats = [];
		}
	}

	// 12-month trend for the corridor.
	let trend: ProcessingTrendPoint[] = [];
	{
		const params = new URLSearchParams({ dest: country.id, visa: visaCategory, months: '12' });
		if (consulate) params.set('consulate', consulate);
		try {
			trend = (await apiList<ProcessingTrendPoint>(ctx, `/visa-timelines/stats/trend?${params}`)).items;
		} catch {
			trend = [];
		}
	}

	// Sibling visa categories for this destination — used for SEO
	// interlinking ("other visa types for {country}").
	let siblingCategories: string[] = [];
	try {
		const all = (await apiList<ProcessingStat>(ctx, `/visa-timelines/stats?dest=${country.id}`)).items;
		siblingCategories = [...new Set(all.map((s) => s.visa_category))]
			.filter((v) => v && v !== visaCategory)
			.sort();
	} catch {
		siblingCategories = [];
	}

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
