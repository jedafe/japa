// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { AMAThread, GeoCountry } from '$lib/types';

const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['forum.ama.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Ask an Expat AMA is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Fetch country spaces for the destination filter dropdown and for
	// resolving host_dest_country_id to a human-readable name (BUG-007).
	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	// Forward ?dest= and ?profession= filters to the backend (BUG-006).
	const url = event.url;
	const params = new URLSearchParams({ page_size: String(PAGE_SIZE) });
	if (url.searchParams.get('dest')) params.set('dest', url.searchParams.get('dest')!);
	if (url.searchParams.get('profession')) params.set('profession', url.searchParams.get('profession')!);

	try {
		const { items } = await apiList<AMAThread>(ctx, `/forum/ama?${params.toString()}`);
		return {
			sessions: items,
			countries,
			authed,
			pageSize: PAGE_SIZE,
			loadError: false
		};
	} catch {
		return {
			sessions: [] as AMAThread[],
			countries,
			authed,
			pageSize: PAGE_SIZE,
			loadError: true
		};
	}
};
