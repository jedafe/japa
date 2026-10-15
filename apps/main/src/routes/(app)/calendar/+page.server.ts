// Ref: japaconnect-engineering/sections/86-community-calendar.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { CalendarEntry, GeoCountry, ListMeta } from '$lib/types';

/**
 * Community Calendar at /calendar — SSR list of calendar entries.
 * Fetches GET /calendar?dest=&category=&from=&to=&view=list,
 * passing through all query params. Default view=list (simplest).
 * On error, degrades to an empty list with loadError: true.
 *
 * view=month|week use a `focus` param (YYYY-MM for month, YYYY-MM-DD for
 * week) to select the displayed period; the API range is derived from it.
 * Explicit from/to params override the focus range.
 */
const PAGE_SIZE = 20;

const VALID_VIEWS = ['list', 'month', 'week'] as const;
type CalendarView = (typeof VALID_VIEWS)[number];

function toISODate(d: Date): string {
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}`;
}

function parseISODate(s: string): Date | null {
	const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s);
	if (!m) return null;
	return new Date(Date.UTC(+m[1], +m[2] - 1, +m[3]));
}

function parseFocusMonth(s: string): Date | null {
	const m = /^(\d{4})-(\d{2})$/.exec(s);
	if (!m) return null;
	return new Date(Date.UTC(+m[1], +m[2] - 1, 1));
}

/** Monday-first week start for the week containing d. */
function weekStart(d: Date): Date {
	const day = d.getUTCDay(); // 0=Sun..6=Sat
	const diff = (day + 6) % 7; // days since Monday
	return new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate() - diff));
}

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
	const focusRaw = event.url.searchParams.get('focus') ?? '';

	const view: CalendarView = VALID_VIEWS.includes(viewRaw as CalendarView)
		? (viewRaw as CalendarView)
		: 'list';

	// BUG-009: fetch categories from the API (calendar.categories system_setting).
	let categories: { value: string; label: string }[] = [];
	try {
		categories = await apiGet<{ value: string; label: string }[]>(ctx, '/calendar/categories');
	} catch {
		categories = [];
	}
	const validCategoryValues = new Set(categories.map((c) => c.value));
	const category = validCategoryValues.has(categoryRaw) ? categoryRaw : '';

	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	// Resolve the API date range and the focused period (month/week views).
	const today = new Date();
	let apiFrom = '';
	let apiTo = '';
	let focus = focusRaw;

	if (view === 'month') {
		// Focused month: ?focus=YYYY-MM, else the month containing `from`, else today.
		const focusMonth =
			parseFocusMonth(focusRaw) ?? parseISODate(from) ?? parseISODate(focusRaw) ?? today;
		const monthStart = new Date(
			Date.UTC(focusMonth.getUTCFullYear(), focusMonth.getUTCMonth(), 1)
		);
		const monthEnd = new Date(
			Date.UTC(focusMonth.getUTCFullYear(), focusMonth.getUTCMonth() + 1, 0)
		);
		focus = `${monthStart.getUTCFullYear()}-${String(monthStart.getUTCMonth() + 1).padStart(2, '0')}`;
		apiFrom = from || toISODate(monthStart);
		apiTo = to || toISODate(monthEnd);
	} else if (view === 'week') {
		// Focused week: ?focus=YYYY-MM-DD, else `from`, else today.
		const focusDay = parseISODate(focusRaw) ?? parseISODate(from) ?? today;
		const ws = weekStart(focusDay);
		const we = new Date(Date.UTC(ws.getUTCFullYear(), ws.getUTCMonth(), ws.getUTCDate() + 6));
		focus = toISODate(ws);
		apiFrom = from || toISODate(ws);
		apiTo = to || toISODate(we);
	} else {
		// List view defaults to upcoming entries from today; preserve explicit
		// URL filters. The raw user-supplied `from` (not the default) is what
		// gets echoed back in filters so hasActiveFilters stays honest.
		apiFrom = from || today.toISOString();
		apiTo = to;
	}

	const params = new URLSearchParams({ view, page_size: String(PAGE_SIZE) });
	if (dest) params.set('dest', dest);
	if (category) params.set('category', category);
	if (apiFrom) params.set('from', apiFrom);
	if (apiTo) params.set('to', apiTo);

	try {
		const { items, meta } = await apiList<CalendarEntry>(ctx, `/calendar?${params.toString()}`);
		const subscribed = !!(meta as (ListMeta & { subscribed?: boolean }) | undefined)?.subscribed;
		return {
			entries: items,
			meta: meta ?? null,
			subscribed,
			countries,
			categories,
			authed,
			pageSize: PAGE_SIZE,
			loadError: false,
			rangeFrom: apiFrom,
			rangeTo: apiTo,
			filters: { dest, category, from, to, view, focus }
		};
	} catch {
		return {
			entries: [] as CalendarEntry[],
			meta: null,
			subscribed: false,
			countries,
			categories,
			authed,
			pageSize: PAGE_SIZE,
			loadError: true,
			rangeFrom: apiFrom,
			rangeTo: apiTo,
			filters: { dest, category, from, to, view, focus }
		};
	}
};
