// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { MemberStage, JapaStage, GeoCountry } from '$lib/types';

const PAGE_SIZE = 20;

/**
 * Japa Stage directory at /japa-stage — lists public member profiles grouped
 * by their japa_stage. Pass-through ?stage filter and ?page to the backend's
 * GET /members/by-stage endpoint.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['japa_stage.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Japa Stage is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const stage = event.url.searchParams.get('stage') ?? '';
	const dest = event.url.searchParams.get('dest') ?? '';
	const page = Number(event.url.searchParams.get('page') ?? '1') || 1;

	let members: MemberStage[] = [];
	let totalPages = 1;
	let loadError = false;
	try {
		const params = new URLSearchParams({ page_size: String(PAGE_SIZE), page: String(page) });
		if (stage) params.set('stage', stage);
		if (dest) params.set('dest', dest);
		const { items, meta } = await apiList<MemberStage>(ctx, `/members/by-stage?${params.toString()}`);
		members = items;
		totalPages = meta?.total_pages ?? 1;
	} catch {
		loadError = true;
	}

	// Load country-level spaces for the destination filter dropdown.
	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	let stages: JapaStage[] = [];
	try {
		const list = await apiGet<{ value: string; label: string }[]>(ctx, '/japa-stage/stages');
		stages = (list ?? []).map((o) => o.value as JapaStage);
	} catch {
		stages = [];
	}

	return {
		members,
		authed,
		countries,
		stages,
		pageSize: PAGE_SIZE,
		currentPage: page,
		totalPages,
		loadError,
		filters: { stage, dest }
	};
};
