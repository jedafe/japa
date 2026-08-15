import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { ArrivalChecklistSelectorEntry, ArrivalChecklistItem, GeoSpaceOption } from '$lib/types';

/**
 * Arrival Checklist — §92, ADR-088.
 * SSR loads the selector from /arrival-checklist/selector which returns the
 * available destinations (only those with published guides). The destination
 * dropdown is built from this data — NOT from /spaces?level=country — because
 * only destinations with guides are relevant here.
 *
 * API: GET /arrival-checklist/selector — available destinations + categories.
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

	// Build the destination dropdown from the selector data (Rule 16 — no
	// hardcoded data). Group by destination_space_id to deduplicate.
	const seen = new Set<string>();
	const destinations: GeoSpaceOption[] = [];
	for (const entry of selectorEntries) {
		if (!seen.has(entry.destination_space_id)) {
			seen.add(entry.destination_space_id);
			destinations.push({
				id: entry.destination_space_id,
				name: entry.destination_space_name ?? entry.destination_space_id,
				slug: entry.destination_space_id,
				code: entry.destination_space_code ?? null
			});
		}
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

	return {
		enabled: true,
		tool,
		selectorEntries,
		destinations,
		items,
		initialDestId,
		user: event.locals.user
	};
};
