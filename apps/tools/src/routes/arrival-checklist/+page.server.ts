import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { ArrivalChecklistSelectorEntry, ArrivalChecklistItem, GeoSpaceOption, UserArrivalChecklist } from '$lib/types';

/**
 * Arrival Checklist — §92, ADR-088.
 * SSR loads the destination dropdown from /geo/countries (all 250 primary
 * geo space countries) and the published guides/categories from
 * /arrival-checklist/selector. The destination dropdown is the geo-aware
 * selector source; selectorEntries remains the source of truth for which
 * destinations have published guides and which categories exist.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /arrival-checklist/selector — available destinations + categories.
 *      GET /arrival-checklist/{guide_id} — checklist items for a guide.
 *
 * Ref: §92 (Arrival Checklist), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when arrival_checklist.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('arrival-checklist')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			selectorEntries: [],
			destinations: [],
			items: [],
			initialDestId: null,
			myChecklists: [],
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// Fetch the selector — this is the source of truth for available
	// destinations (only those with published guides appear here).
	let selectorEntries: ArrivalChecklistSelectorEntry[] = [];
	try {
		selectorEntries = (
			await apiList<ArrivalChecklistSelectorEntry>(ctx, '/arrival-checklist/selector')
		).items;
	} catch {
		selectorEntries = [];
	}

	// Destination country list — the full 250 primary geo space countries.
	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = await loadCountries(ctx);
	} catch {
		destinations = [];
	}

	// If ?dest= is present and valid, fetch the guide items for that
	// destination so the page renders with content on first paint (SSR).
	// The selector returns guide_id for each dest+visa combo — use it to
	// fetch items via GET /arrival-checklist/{guide_id}.
	const destParam = event.url.searchParams.get('dest');
	const initialDestId =
		destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

	let items: ArrivalChecklistItem[] = [];
	if (initialDestId) {
		const firstEntry = selectorEntries.find((e) => e.destination_space_id === initialDestId);
		if (firstEntry?.guide_id) {
			try {
				items = (await apiGet<ArrivalChecklistItem[]>(ctx, `/arrival-checklist/${firstEntry.guide_id}`)) ?? [];
			} catch {
				items = [];
			}
		}
	}

	// The user's in-progress checklists (§92 — GET /me/arrival-checklists).
	// Powers the "your checklists" strip so returning users can resume.
	let myChecklists: UserArrivalChecklist[] = [];
	if (event.locals.user) {
		try {
			myChecklists = (await apiList<UserArrivalChecklist>(ctx, '/me/arrival-checklists')).items;
		} catch {
			myChecklists = [];
		}
	}

	return {
		enabled: true,
		tool,
		selectorEntries,
		destinations,
		items,
		initialDestId,
		myChecklists,
		user: event.locals.user
	};
};
