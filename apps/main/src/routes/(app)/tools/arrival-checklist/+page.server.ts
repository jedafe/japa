import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { ArrivalChecklistSelectorEntry, ArrivalChecklistItem, GeoSpaceOption, UserArrivalChecklist } from '$lib/types';

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
 * Route: /tools/arrival-checklist
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['arrival_checklist.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Arrival Checklist is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let selector: ArrivalChecklistSelectorEntry[] = [];
	let destinations: GeoSpaceOption[] = [];
	let disabled = false;
	try {
		selector = (
			await apiList<ArrivalChecklistSelectorEntry>(ctx, '/arrival-checklist/selector')
		).items;
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.code === 'arrival_checklist_disabled')) {
			disabled = true;
		}
		// Other errors: leave selector empty; page shows an empty state.
	}

	// Build the destination dropdown from the selector data (Rule 16 — no
	// hardcoded data). Group by destination_space_id to deduplicate.
	const seen = new Set<string>();
	for (const entry of selector) {
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
	if (initialDestId && !disabled) {
		const firstEntry = selector.find((e) => e.destination_space_id === initialDestId);
		if (firstEntry?.guide_id) {
			try {
				items = (await apiGet<ArrivalChecklistItem[]>(ctx, `/arrival-checklist/${firstEntry.guide_id}`)) ?? [];
			} catch {
				items = [];
			}
		}
	}

	let myChecklists: UserArrivalChecklist[] = [];
	if (authed && !disabled) {
		try {
			myChecklists = (await apiList<UserArrivalChecklist>(ctx, '/me/arrival-checklists')).items;
		} catch {
			myChecklists = [];
		}
	}

	return { selector, destinations, items, initialDestId, disabled, authed, myChecklists };
};
