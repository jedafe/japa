// Ref: japaconnect-engineering/sections/86-community-calendar.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { CalendarEntry, CalendarCategory } from '$lib/types';

/**
 * Community Calendar at /calendar — SSR list of calendar entries.
 * Fetches GET /calendar?dest=&origin=&category=&from=&to=&view=list,
 * passing through all query params. Default view=list (simplest).
 * On error, degrades to an empty list with loadError: true.
 */
const PAGE_SIZE = 20;

const VALID_CATEGORIES: CalendarCategory[] = [
	'visa_window',
	'embassy_closure',
	'application_deadline',
	'exam_window',
	'immigration_office',
	'other'
];

const VALID_VIEWS = ['list', 'month', 'week'] as const;
type CalendarView = (typeof VALID_VIEWS)[number];

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const dest = event.url.searchParams.get('dest') ?? '';
	const origin = event.url.searchParams.get('origin') ?? '';
	const categoryRaw = event.url.searchParams.get('category') ?? '';
	const from = event.url.searchParams.get('from') ?? '';
	const to = event.url.searchParams.get('to') ?? '';
	const viewRaw = event.url.searchParams.get('view') ?? 'list';

	const category = VALID_CATEGORIES.includes(categoryRaw as CalendarCategory)
		? (categoryRaw as CalendarCategory)
		: '';
	const view: CalendarView = VALID_VIEWS.includes(viewRaw as CalendarView)
		? (viewRaw as CalendarView)
		: 'list';

	const params = new URLSearchParams({ view, page_size: String(PAGE_SIZE) });
	if (dest) params.set('dest', dest);
	if (origin) params.set('origin', origin);
	if (category) params.set('category', category);
	if (from) params.set('from', from);
	if (to) params.set('to', to);

	try {
		const { items } = await apiList<CalendarEntry>(ctx, `/calendar?${params.toString()}`);
		return { entries: items, authed, pageSize: PAGE_SIZE, loadError: false, filters: { dest, origin, category, from, to, view } };
	} catch {
		return {
			entries: [] as CalendarEntry[],
			authed,
			pageSize: PAGE_SIZE,
			loadError: true,
			filters: { dest, origin, category, from, to, view }
		};
	}
};
