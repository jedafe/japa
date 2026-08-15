// Ref: japaconnect-engineering/sections/86-community-calendar.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { CalendarEntry, CalendarCategory, Space } from '$lib/types';

/**
 * Community Calendar at /calendar — SSR list of calendar entries.
 * Fetches GET /calendar?dest=&category=&from=&to=&view=list,
 * passing through all query params. Default view=list (simplest).
 * On error, degrades to an empty list with loadError: true.
 */
const PAGE_SIZE = 20;

const VALID_VIEWS = ['list', 'month', 'week'] as const;
type CalendarView = (typeof VALID_VIEWS)[number];

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['calendar.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Community Calendar is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const dest = event.url.searchParams.get('dest') ?? '';
	const categoryRaw = event.url.searchParams.get('category') ?? '';
	const from = event.url.searchParams.get('from') ?? '';
	const to = event.url.searchParams.get('to') ?? '';
	const viewRaw = event.url.searchParams.get('view') ?? 'list';

	const view: CalendarView = VALID_VIEWS.includes(viewRaw as CalendarView)
		? (viewRaw as CalendarView)
		: 'list';

	// BUG-009: fetch categories from the API (calendar.categories system_setting).
	let categories: { value: string; label: string }[] = [];
	try {
		const resp = await apiGet<{ data: { value: string; label: string }[] }>(ctx, '/calendar/categories');
		categories = resp.data ?? [];
	} catch {
		categories = [];
	}
	const validCategoryValues = new Set(categories.map((c) => c.value));
	const category = validCategoryValues.has(categoryRaw) ? categoryRaw : '';

	let countries: Space[] = [];
	try {
		const { items } = await apiList<Space>(ctx, '/spaces?level=country');
		countries = items;
	} catch {
		countries = [];
	}

	const params = new URLSearchParams({ view, page_size: String(PAGE_SIZE) });
	if (dest) params.set('dest', dest);
	if (category) params.set('category', category);
	if (from) params.set('from', from);
	if (to) params.set('to', to);

	try {
		const { items } = await apiList<CalendarEntry>(ctx, `/calendar?${params.toString()}`);
		return { entries: items, countries, categories, authed, pageSize: PAGE_SIZE, loadError: false, filters: { dest, category, from, to, view } };
	} catch {
		return {
			entries: [] as CalendarEntry[],
			countries,
			categories,
			authed,
			pageSize: PAGE_SIZE,
			loadError: true,
			filters: { dest, category, from, to, view }
		};
	}
};
